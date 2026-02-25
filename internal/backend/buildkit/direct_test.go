package buildkit

import (
	"testing"

	"github.com/Makepad-fr/buildgraph/internal/backend"
	bkclient "github.com/moby/buildkit/client"
)

func TestConfigureExportsLocalUsesOutputDir(t *testing.T) {
	t.Parallel()

	solveOpt := &bkclient.SolveOpt{}
	outputs, warnings, err := configureExports(solveOpt, backend.BuildRequest{
		OutputMode: backend.OutputLocal,
		LocalDest:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("configure exports: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if len(outputs) != 1 {
		t.Fatalf("expected single output, got %d", len(outputs))
	}
	if len(solveOpt.Exports) != 1 {
		t.Fatalf("expected single export entry, got %d", len(solveOpt.Exports))
	}
	export := solveOpt.Exports[0]
	if export.Type != bkclient.ExporterLocal {
		t.Fatalf("expected local exporter, got %q", export.Type)
	}
	if export.OutputDir == "" {
		t.Fatalf("expected OutputDir to be set")
	}
	if export.Attrs != nil {
		t.Fatalf("expected Attrs to be nil for local exporter, got %v", export.Attrs)
	}
}
