package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Makepad-fr/buildgraph/internal/backend"
	buildkitbackend "github.com/Makepad-fr/buildgraph/internal/backend/buildkit"
)

func TestDirectBuildkitBuildLocalOutput(t *testing.T) {
	endpoint := os.Getenv("BUILDGRAPH_INTEGRATION_DIRECT_ENDPOINT")
	if endpoint == "" {
		t.Skip("set BUILDGRAPH_INTEGRATION_DIRECT_ENDPOINT to run direct BuildKit integration test")
	}

	ctx := context.Background()
	be := buildkitbackend.NewBackend()
	outDir := t.TempDir()
	fixtures := filepath.Join("fixtures")

	_, err := be.Build(ctx, backend.BuildRequest{
		ContextDir: fixtures,
		Dockerfile: "Dockerfile.integration",
		OutputMode: backend.OutputLocal,
		LocalDest:  outDir,
		Endpoint:   endpoint,
	}, nil)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "hello.txt")); err != nil {
		t.Fatalf("expected exported file hello.txt: %v", err)
	}
}

func TestDockerBackedBuildImageOutput(t *testing.T) {
	if os.Getenv("BUILDGRAPH_INTEGRATION_DOCKER") != "1" {
		t.Skip("set BUILDGRAPH_INTEGRATION_DOCKER=1 to run docker-backed integration test")
	}

	ctx := context.Background()
	be := buildkitbackend.NewBackend()
	fixtures := filepath.Join("fixtures")

	_, err := be.Build(ctx, backend.BuildRequest{
		ContextDir: fixtures,
		Dockerfile: "Dockerfile.integration",
		OutputMode: backend.OutputImage,
		ImageRef:   "buildgraph/integration:test",
	}, nil)
	if err != nil {
		t.Fatalf("docker-backed build failed: %v", err)
	}
}
