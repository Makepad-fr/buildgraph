package buildkit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Makepad-fr/buildgraph/internal/backend"
	bkclient "github.com/moby/buildkit/client"
	bksession "github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"golang.org/x/sync/errgroup"
)

type DirectDriver struct{}

func NewDirectDriver() *DirectDriver {
	return &DirectDriver{}
}

func (d *DirectDriver) Ping(ctx context.Context, endpoint string) error {
	client, err := bkclient.New(ctx, endpoint)
	if err != nil {
		return err
	}
	defer client.Close()
	_, err = client.ListWorkers(ctx)
	return err
}

func (d *DirectDriver) Build(ctx context.Context, endpoint string, req backend.BuildRequest, progressFn backend.BuildProgressFunc) (backend.BuildResult, error) {
	if endpoint == "" {
		return backend.BuildResult{}, fmt.Errorf("buildkit endpoint is required")
	}
	client, err := bkclient.New(ctx, endpoint)
	if err != nil {
		return backend.BuildResult{}, fmt.Errorf("connect buildkit: %w", err)
	}
	defer client.Close()

	contextDir, _, dockerfileDir, dockerfileName, err := normalizePaths(req.ContextDir, req.Dockerfile)
	if err != nil {
		return backend.BuildResult{}, err
	}

	frontendAttrs := map[string]string{
		"filename": dockerfileName,
	}
	if req.Target != "" {
		frontendAttrs["target"] = req.Target
	}
	if len(req.Platforms) > 0 {
		frontendAttrs["platform"] = strings.Join(req.Platforms, ",")
	}
	for _, arg := range req.BuildArgs {
		frontendAttrs["build-arg:"+arg.Key] = arg.Value
	}

	solveOpt := bkclient.SolveOpt{
		Frontend:      "dockerfile.v0",
		FrontendAttrs: frontendAttrs,
		LocalDirs: map[string]string{
			"context":    contextDir,
			"dockerfile": dockerfileDir,
		},
		Session: []bksession.Attachable{},
	}
	if len(req.Secrets) > 0 {
		sources := make([]secretsprovider.Source, 0, len(req.Secrets))
		for _, secret := range req.Secrets {
			if secret.ID == "" {
				continue
			}
			source := secretsprovider.Source{ID: secret.ID}
			if secret.Src != "" {
				source.FilePath = secret.Src
			} else {
				source.Env = secret.ID
			}
			sources = append(sources, source)
		}
		if len(sources) > 0 {
			store, err := secretsprovider.NewStore(sources)
			if err != nil {
				return backend.BuildResult{}, fmt.Errorf("configure secrets provider: %w", err)
			}
			solveOpt.Session = append(solveOpt.Session, secretsprovider.NewSecretProvider(store))
		}
	}

	outputs, warnings, err := configureExports(&solveOpt, req)
	if err != nil {
		return backend.BuildResult{}, err
	}

	statusCh := make(chan *bkclient.SolveStatus)
	var cacheHits int
	var cacheMisses int

	progressCtx, cancelProgress := context.WithCancel(ctx)
	defer cancelProgress()
	group, groupCtx := errgroup.WithContext(progressCtx)
	group.Go(func() error {
		for {
			select {
			case <-groupCtx.Done():
				return groupCtx.Err()
			case status, ok := <-statusCh:
				if !ok {
					return nil
				}
				for _, vertex := range status.Vertexes {
					if vertex == nil {
						continue
					}
					state := "running"
					if vertex.Completed != nil {
						state = "completed"
					}
					if vertex.Cached {
						cacheHits++
					} else {
						cacheMisses++
					}
					if progressFn != nil {
						progressFn(backend.BuildProgressEvent{
							Timestamp: time.Now().UTC(),
							Phase:     "build",
							Message:   vertex.Name,
							VertexID:  vertex.Digest.String(),
							Status:    state,
						})
					}
				}
			}
		}
	})

	response, solveErr := client.Solve(ctx, nil, solveOpt, statusCh)
	// BuildKit closes statusCh itself; cancel progress context to ensure
	// consumer goroutine exits even if the channel is not closed on error paths.
	cancelProgress()
	progressErr := group.Wait()
	if progressErr != nil && !errors.Is(progressErr, context.Canceled) {
		return backend.BuildResult{}, fmt.Errorf("consume build progress: %w", progressErr)
	}
	if solveErr != nil {
		return backend.BuildResult{}, fmt.Errorf("buildkit solve: %w", solveErr)
	}

	digest := response.ExporterResponse["containerimage.digest"]
	if digest == "" {
		digest = response.ExporterResponse["containerimage.config.digest"]
	}

	return backend.BuildResult{
		Outputs:             outputs,
		Digest:              digest,
		ProvenanceAvailable: response.ExporterResponse["attestation-manifest"] != "",
		CacheStats: backend.CacheStats{
			Hits:   cacheHits,
			Misses: cacheMisses,
		},
		Warnings:         warnings,
		ExporterResponse: response.ExporterResponse,
	}, nil
}

func configureExports(solveOpt *bkclient.SolveOpt, req backend.BuildRequest) ([]string, []string, error) {
	switch req.OutputMode {
	case "", backend.OutputImage:
		if req.ImageRef == "" {
			return nil, nil, fmt.Errorf("--image-ref is required for image output")
		}
		solveOpt.Exports = []bkclient.ExportEntry{{
			Type: bkclient.ExporterImage,
			Attrs: map[string]string{
				"name": req.ImageRef,
				"push": "false",
			},
		}}
		return []string{req.ImageRef}, nil, nil
	case backend.OutputOCI:
		if req.OCIDest == "" {
			return nil, nil, fmt.Errorf("--oci-dest is required for oci output")
		}
		if err := os.MkdirAll(filepath.Dir(req.OCIDest), 0o755); err != nil {
			return nil, nil, fmt.Errorf("create oci destination dir: %w", err)
		}
		solveOpt.Exports = []bkclient.ExportEntry{{
			Type: bkclient.ExporterOCI,
			Output: func(_ map[string]string) (io.WriteCloser, error) {
				return os.Create(req.OCIDest)
			},
		}}
		return []string{req.OCIDest}, nil, nil
	case backend.OutputLocal:
		if req.LocalDest == "" {
			return nil, nil, fmt.Errorf("--local-dest is required for local output")
		}
		if err := os.MkdirAll(req.LocalDest, 0o755); err != nil {
			return nil, nil, fmt.Errorf("create local destination: %w", err)
		}
		solveOpt.Exports = []bkclient.ExportEntry{{
			Type: bkclient.ExporterLocal,
			Attrs: map[string]string{
				"dest": req.LocalDest,
			},
		}}
		return []string{req.LocalDest}, nil, nil
	default:
		return nil, nil, fmt.Errorf("unsupported output mode %q", req.OutputMode)
	}
}

func normalizePaths(contextDir, dockerfile string) (string, string, string, string, error) {
	if contextDir == "" {
		contextDir = "."
	}
	contextAbs, err := filepath.Abs(contextDir)
	if err != nil {
		return "", "", "", "", fmt.Errorf("resolve context path: %w", err)
	}
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	dockerfilePath := dockerfile
	if !filepath.IsAbs(dockerfilePath) {
		dockerfilePath = filepath.Join(contextAbs, dockerfile)
	}
	if _, err := os.Stat(dockerfilePath); err != nil {
		return "", "", "", "", fmt.Errorf("dockerfile not found: %w", err)
	}
	return contextAbs, dockerfilePath, filepath.Dir(dockerfilePath), filepath.Base(dockerfilePath), nil
}
