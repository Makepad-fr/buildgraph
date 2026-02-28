package buildkit

import (
	"testing"
	"time"

	"github.com/Makepad-fr/buildgraph/internal/backend"
	bkclient "github.com/moby/buildkit/client"
	"github.com/opencontainers/go-digest"
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

func TestToBuildProgressEventMapsGraphFields(t *testing.T) {
	t.Parallel()

	started := time.Unix(10, 0).UTC()
	completed := time.Unix(12, 0).UTC()
	vertex := &bkclient.Vertex{
		Digest:    digest.FromString("vertex"),
		Inputs:    []digest.Digest{digest.FromString("input-a"), digest.FromString("input-b")},
		Name:      "RUN apk add curl",
		Started:   &started,
		Completed: &completed,
		Cached:    true,
		Error:     "boom",
	}

	event := toBuildProgressEvent(vertex)
	if got, want := event.VertexID, vertex.Digest.String(); got != want {
		t.Fatalf("unexpected vertex id: got=%q want=%q", got, want)
	}
	if got, want := len(event.Inputs), 2; got != want {
		t.Fatalf("unexpected input count: got=%d want=%d", got, want)
	}
	if event.Started == nil || !event.Started.Equal(started) {
		t.Fatalf("unexpected started time: %v", event.Started)
	}
	if event.Completed == nil || !event.Completed.Equal(completed) {
		t.Fatalf("unexpected completed time: %v", event.Completed)
	}
	if !event.Cached {
		t.Fatalf("expected cached=true")
	}
	if got, want := event.Error, "boom"; got != want {
		t.Fatalf("unexpected error: got=%q want=%q", got, want)
	}
	if got, want := event.Status, "completed"; got != want {
		t.Fatalf("unexpected status: got=%q want=%q", got, want)
	}
}
