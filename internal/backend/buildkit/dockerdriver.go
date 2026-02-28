package buildkit

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Makepad-fr/buildgraph/internal/backend"
	ctrplatforms "github.com/containerd/platforms"
	dockerconfig "github.com/docker/cli/cli/config"
	bksession "github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	buildtypes "github.com/moby/moby/api/types/build"
	dockerclient "github.com/moby/moby/client"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"
)

type DockerDriver struct{}

func NewDockerDriver() *DockerDriver {
	return &DockerDriver{}
}

func (d *DockerDriver) Ping(ctx context.Context) error {
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer cli.Close()
	_, err = cli.Ping(ctx, dockerclient.PingOptions{})
	return err
}

func (d *DockerDriver) Build(ctx context.Context, req backend.BuildRequest, progressFn backend.BuildProgressFunc) (backend.BuildResult, error) {
	if req.OutputMode != "" && req.OutputMode != backend.OutputImage {
		return backend.BuildResult{}, fmt.Errorf("docker-backed mode currently supports only image output; use direct BuildKit endpoint for %s", req.OutputMode)
	}
	if req.ImageRef == "" {
		return backend.BuildResult{}, fmt.Errorf("--image-ref is required for image output")
	}

	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return backend.BuildResult{}, fmt.Errorf("create docker client: %w", err)
	}
	defer cli.Close()

	contextPath := req.ContextDir
	if contextPath == "" {
		contextPath = "."
	}
	contextPath, err = filepath.Abs(contextPath)
	if err != nil {
		return backend.BuildResult{}, fmt.Errorf("resolve context path: %w", err)
	}
	dockerfile := req.Dockerfile
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	dockerfilePath := dockerfile
	if !filepath.IsAbs(dockerfilePath) {
		dockerfilePath = filepath.Join(contextPath, dockerfile)
	}
	if _, err := os.Stat(dockerfilePath); err != nil {
		return backend.BuildResult{}, fmt.Errorf("dockerfile not found: %w", err)
	}
	dockerfileRelative, err := filepath.Rel(contextPath, dockerfilePath)
	if err != nil {
		return backend.BuildResult{}, fmt.Errorf("resolve dockerfile path: %w", err)
	}
	if strings.HasPrefix(dockerfileRelative, "..") {
		return backend.BuildResult{}, fmt.Errorf("dockerfile must be inside build context for docker-backed mode")
	}

	archiveReader, err := tarContextDirectory(contextPath)
	if err != nil {
		return backend.BuildResult{}, fmt.Errorf("create build context archive: %w", err)
	}
	defer archiveReader.Close()

	buildArgs := map[string]*string{}
	for _, arg := range req.BuildArgs {
		value := arg.Value
		buildArgs[arg.Key] = &value
	}

	options := dockerclient.ImageBuildOptions{
		Dockerfile: filepath.ToSlash(dockerfileRelative),
		Tags:       []string{req.ImageRef},
		BuildArgs:  buildArgs,
		Target:     req.Target,
		Version:    buildtypes.BuilderBuildKit,
		Remove:     true,
	}
	session, err := startDockerBuildSession(ctx, cli)
	if err != nil {
		return backend.BuildResult{}, err
	}
	defer session.Close()
	options.SessionID = session.ID()

	if len(req.Platforms) > 0 {
		options.Platforms = make([]specs.Platform, 0, len(req.Platforms))
		for _, platform := range req.Platforms {
			parsed, err := ctrplatforms.Parse(platform)
			if err != nil {
				return backend.BuildResult{}, fmt.Errorf("invalid platform %q: %w", platform, err)
			}
			options.Platforms = append(options.Platforms, parsed)
		}
	}
	response, err := cli.ImageBuild(ctx, archiveReader, options)
	if err != nil {
		return backend.BuildResult{}, fmt.Errorf("docker image build: %w", err)
	}
	defer response.Body.Close()

	decoder := json.NewDecoder(response.Body)
	warnings := make([]string, 0)
	for decoder.More() {
		var msg dockerBuildMessage
		if err := decoder.Decode(&msg); err != nil {
			break
		}
		if msg.ErrorDetail.Message != "" {
			return backend.BuildResult{}, fmt.Errorf("docker build error: %s", msg.ErrorDetail.Message)
		}
		if msg.Error != "" {
			return backend.BuildResult{}, fmt.Errorf("docker build error: %s", msg.Error)
		}
		text := strings.TrimSpace(msg.Stream)
		if text == "" {
			text = strings.TrimSpace(msg.Status)
		}
		if text != "" && progressFn != nil {
			progressFn(backend.BuildProgressEvent{
				Timestamp: time.Now().UTC(),
				Phase:     "build",
				Message:   text,
				Status:    "running",
			})
		}
		if strings.Contains(strings.ToLower(text), "warning") {
			warnings = append(warnings, text)
		}
	}

	inspect, err := cli.ImageInspect(ctx, req.ImageRef)
	if err != nil {
		return backend.BuildResult{}, fmt.Errorf("inspect built image: %w", err)
	}

	return backend.BuildResult{
		Outputs: []string{req.ImageRef},
		Digest:  inspect.ID,
		CacheStats: backend.CacheStats{
			Hits:   0,
			Misses: 0,
		},
		Warnings: warnings,
	}, nil
}

type dockerBuildMessage struct {
	Stream      string `json:"stream"`
	Status      string `json:"status"`
	Error       string `json:"error"`
	ErrorDetail struct {
		Message string `json:"message"`
	} `json:"errorDetail"`
}

func tarContextDirectory(root string) (io.ReadCloser, error) {
	pr, pw := io.Pipe()

	go func() {
		tw := tar.NewWriter(pw)
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if path == root {
				return nil
			}

			info, err := d.Info()
			if err != nil {
				return err
			}
			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			header.Name = filepath.ToSlash(relative)
			if info.IsDir() {
				header.Name += "/"
			}
			if err := tw.WriteHeader(header); err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			_, err = io.Copy(tw, file)
			return err
		})

		closeErr := tw.Close()
		if err == nil {
			err = closeErr
		}
		_ = pw.CloseWithError(err)
	}()

	return pr, nil
}

type dockerBuildSession struct {
	session *bksession.Session
	cancel  context.CancelFunc
	group   *errgroup.Group
}

func startDockerBuildSession(ctx context.Context, cli *dockerclient.Client) (*dockerBuildSession, error) {
	// BuildKit frontend resolution on the daemon expects a live session; wire it
	// through Docker's /session hijack endpoint.
	session, err := bksession.NewSession(ctx, "buildgraph")
	if err != nil {
		return nil, fmt.Errorf("create docker build session: %w", err)
	}
	dockerCfg := dockerconfig.LoadDefaultConfigFile(io.Discard)
	session.Allow(authprovider.NewDockerAuthProvider(authprovider.DockerAuthProviderConfig{
		AuthConfigProvider: authprovider.LoadAuthConfig(dockerCfg),
	}))
	runCtx, cancel := context.WithCancel(ctx)
	group, groupCtx := errgroup.WithContext(runCtx)
	group.Go(func() error {
		return session.Run(groupCtx, func(ctx context.Context, proto string, meta map[string][]string) (net.Conn, error) {
			return cli.DialHijack(ctx, "/session", proto, meta)
		})
	})
	return &dockerBuildSession{
		session: session,
		cancel:  cancel,
		group:   group,
	}, nil
}

func (s *dockerBuildSession) ID() string {
	if s == nil || s.session == nil {
		return ""
	}
	return s.session.ID()
}

func (s *dockerBuildSession) Close() error {
	if s == nil {
		return nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.session != nil {
		_ = s.session.Close()
	}
	if s.group == nil {
		return nil
	}
	err := s.group.Wait()
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
