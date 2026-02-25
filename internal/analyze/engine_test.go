package analyze

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Makepad-fr/buildgraph/internal/backend"
)

func TestAnalyzeBadDockerfileHasFindings(t *testing.T) {
	t.Parallel()
	engine := NewEngine()
	ctx := context.Background()

	findings, err := engine.Analyze(ctx, backend.AnalyzeRequest{
		ContextDir: filepath.Join("..", "..", "integration", "fixtures"),
		Dockerfile: "Dockerfile.bad",
	})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if len(findings) == 0 {
		t.Fatalf("expected findings for bad Dockerfile")
	}
}

func TestAnalyzeGoodDockerfileProducesFewerFindings(t *testing.T) {
	t.Parallel()
	engine := NewEngine()
	ctx := context.Background()

	badFindings, err := engine.Analyze(ctx, backend.AnalyzeRequest{
		ContextDir: filepath.Join("..", "..", "integration", "fixtures"),
		Dockerfile: "Dockerfile.bad",
	})
	if err != nil {
		t.Fatalf("analyze bad failed: %v", err)
	}
	goodFindings, err := engine.Analyze(ctx, backend.AnalyzeRequest{
		ContextDir: filepath.Join("..", "..", "integration", "fixtures"),
		Dockerfile: "Dockerfile.good",
	})
	if err != nil {
		t.Fatalf("analyze good failed: %v", err)
	}
	if len(goodFindings) >= len(badFindings) {
		t.Fatalf("expected good Dockerfile to have fewer findings: good=%d bad=%d", len(goodFindings), len(badFindings))
	}
}
