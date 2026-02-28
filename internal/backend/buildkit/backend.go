package buildkit

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/Makepad-fr/buildgraph/internal/analyze"
	"github.com/Makepad-fr/buildgraph/internal/backend"
	"gopkg.in/yaml.v3"
)

const BackendName = "buildkit"

type Backend struct {
	analyzer *analyze.Engine
	direct   directClient
	docker   dockerClient
}

type directClient interface {
	Ping(ctx context.Context, endpoint string) error
	Build(ctx context.Context, endpoint string, req backend.BuildRequest, progress backend.BuildProgressFunc) (backend.BuildResult, error)
}

type dockerClient interface {
	Ping(ctx context.Context) error
	Build(ctx context.Context, req backend.BuildRequest, progress backend.BuildProgressFunc) (backend.BuildResult, error)
}

func NewBackend() *Backend {
	return &Backend{
		analyzer: analyze.NewEngine(),
		direct:   NewDirectDriver(),
		docker:   NewDockerDriver(),
	}
}

func (b *Backend) Name() string {
	return BackendName
}

func (b *Backend) Detect(ctx context.Context, req backend.DetectRequest) (backend.DetectResult, error) {
	resolved, err := b.resolveEndpoint(ctx, req.Endpoint, req.ProjectConfigPath, req.GlobalConfigPath)
	if err != nil {
		return backend.DetectResult{
			Backend:   BackendName,
			Endpoint:  "",
			Available: false,
			Mode:      "none",
			Details:   err.Error(),
			Attempts:  resolved.Attempts,
			Metadata:  map[string]string{},
		}, err
	}

	return backend.DetectResult{
		Backend:   BackendName,
		Endpoint:  resolved.Endpoint,
		Mode:      resolved.Mode,
		Available: true,
		Details:   resolved.Details,
		Attempts:  resolved.Attempts,
		Metadata: map[string]string{
			"resolutionSource": resolved.Source,
		},
	}, nil
}

func (b *Backend) Analyze(ctx context.Context, req backend.AnalyzeRequest) (backend.AnalyzeResult, error) {
	findings, err := b.analyzer.Analyze(ctx, req)
	if err != nil {
		return backend.AnalyzeResult{}, err
	}

	filtered := make([]backend.Finding, 0, len(findings))
	for _, finding := range findings {
		if backend.FindingMeetsThreshold(finding, req.SeverityThreshold) {
			filtered = append(filtered, finding)
		}
	}

	detectResult, err := b.Detect(ctx, backend.DetectRequest{
		Backend:           req.Backend,
		Endpoint:          req.Endpoint,
		ProjectConfigPath: req.ProjectConfigPath,
		GlobalConfigPath:  req.GlobalConfigPath,
	})
	if err != nil {
		detectResult = backend.DetectResult{
			Backend:  BackendName,
			Endpoint: "",
			Mode:     "none",
			Details:  "build backend detection failed; static analysis still completed",
		}
	}

	return backend.AnalyzeResult{
		Backend:  BackendName,
		Endpoint: detectResult.Endpoint,
		Findings: filtered,
	}, nil
}

func (b *Backend) Build(ctx context.Context, req backend.BuildRequest, progress backend.BuildProgressFunc) (backend.BuildResult, error) {
	resolved, err := b.resolveEndpoint(ctx, req.Endpoint, req.ProjectConfigPath, req.GlobalConfigPath)
	if err != nil {
		return backend.BuildResult{}, fmt.Errorf("detect build endpoint: %w", err)
	}

	switch resolved.Mode {
	case "direct":
		result, err := b.direct.Build(ctx, resolved.Endpoint, req, progress)
		if err != nil {
			return backend.BuildResult{}, err
		}
		result.Backend = BackendName
		result.Endpoint = resolved.Endpoint
		return result, nil
	case "docker":
		result, err := b.docker.Build(ctx, req, progress)
		if err != nil {
			return backend.BuildResult{}, err
		}
		result.Backend = BackendName
		result.Endpoint = resolved.Endpoint
		return result, nil
	default:
		return backend.BuildResult{}, fmt.Errorf("unsupported build mode %q", resolved.Mode)
	}
}

func (b *Backend) Capabilities(_ context.Context) (backend.BackendCapabilities, error) {
	return backend.BackendCapabilities{
		SupportsAnalyze:       true,
		SupportsImageOutput:   true,
		SupportsOCIOutput:     true,
		SupportsLocalOutput:   true,
		SupportsRemoteBuild:   true,
		SupportsProgressEvent: true,
	}, nil
}

type endpointResolution struct {
	Endpoint string
	Mode     string
	Source   string
	Details  string
	Attempts []backend.DetectAttempt
}

func (b *Backend) resolveEndpoint(ctx context.Context, explicit, projectConfigPath, globalConfigPath string) (endpointResolution, error) {
	attempts := make([]backend.DetectAttempt, 0, 8)

	tryDirect := func(source, endpoint, details string) (endpointResolution, bool) {
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" {
			return endpointResolution{}, false
		}
		attempt := backend.DetectAttempt{
			Source:   source,
			Endpoint: endpoint,
			Mode:     "direct",
			Details:  details,
		}
		if err := b.direct.Ping(ctx, endpoint); err != nil {
			attempt.Status = "error"
			attempt.Error = err.Error()
			attempts = append(attempts, attempt)
			return endpointResolution{}, false
		}
		attempt.Status = "ok"
		attempts = append(attempts, attempt)
		return endpointResolution{
			Endpoint: endpoint,
			Mode:     "direct",
			Source:   source,
			Details:  details,
			Attempts: attempts,
		}, true
	}

	tryDocker := func(source, details string) (endpointResolution, bool) {
		attempt := backend.DetectAttempt{
			Source:   source,
			Endpoint: "docker://local",
			Mode:     "docker",
			Details:  details,
		}
		if err := b.docker.Ping(ctx); err != nil {
			attempt.Status = "error"
			attempt.Error = err.Error()
			attempts = append(attempts, attempt)
			return endpointResolution{}, false
		}
		attempt.Status = "ok"
		attempts = append(attempts, attempt)
		return endpointResolution{
			Endpoint: "docker://local",
			Mode:     "docker",
			Source:   source,
			Details:  details,
			Attempts: attempts,
		}, true
	}

	if resolved, ok := tryDirect("flag", explicit, "using explicit BuildKit endpoint"); ok {
		return resolved, nil
	}

	if resolved, ok := tryDirect("env", os.Getenv("BUILDKIT_HOST"), "using BUILDKIT_HOST"); ok {
		return resolved, nil
	}

	if resolved, ok := tryDirect("project-config", readEndpointFromConfig(projectConfigPath), "using project config endpoint"); ok {
		return resolved, nil
	}
	if resolved, ok := tryDirect("global-config", readEndpointFromConfig(globalConfigPath), "using global config endpoint"); ok {
		return resolved, nil
	}

	if runtime.GOOS == "windows" {
		if resolved, ok := tryDocker("auto", "docker daemon reachable"); ok {
			return resolved, nil
		}
		for _, endpoint := range windowsDefaultEndpoints() {
			if resolved, ok := tryDirect("auto", endpoint, "direct BuildKit endpoint discovered"); ok {
				return resolved, nil
			}
		}
		return endpointResolution{Attempts: attempts}, fmt.Errorf("no BuildKit endpoint detected on Windows: docker daemon unavailable and no direct endpoint reachable")
	}

	for _, endpoint := range unixDefaultEndpoints() {
		if resolved, ok := tryDirect("auto", endpoint, "direct BuildKit endpoint discovered"); ok {
			return resolved, nil
		}
	}
	if resolved, ok := tryDocker("auto", "docker daemon reachable"); ok {
		return resolved, nil
	}

	return endpointResolution{Attempts: attempts}, fmt.Errorf("no BuildKit endpoint detected")
}

func readEndpointFromConfig(path string) string {
	if path == "" {
		return ""
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var cfg struct {
		Endpoint string `yaml:"endpoint"`
	}
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.Endpoint)
}

func unixDefaultEndpoints() []string {
	return []string{
		"unix:///run/buildkit/buildkitd.sock",
		"unix:///var/run/buildkit/buildkitd.sock",
		"unix:///Users/runner/buildkit/buildkitd.sock",
	}
}

func windowsDefaultEndpoints() []string {
	return []string{
		"npipe:////./pipe/buildkitd",
	}
}
