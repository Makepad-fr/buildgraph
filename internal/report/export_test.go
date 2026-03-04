package report

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Makepad-fr/buildgraph/internal/backend"
)

func TestRenderDOTIncludesGraph(t *testing.T) {
	t.Parallel()
	run := BuildReport{
		Build: backend.BuildResult{
			Vertices: []backend.BuildVertex{
				{ID: "v1", Name: "RUN apt-get update", DurationMS: 40, Cached: false},
				{ID: "v2", Name: "RUN go build", DurationMS: 90, Cached: true},
			},
			Edges: []backend.BuildEdge{{From: "v1", To: "v2"}},
		},
		Metrics: backend.BuildMetrics{CriticalPathVertices: []string{"v1", "v2"}},
	}
	dot := RenderDOT(run)
	if !strings.Contains(dot, "digraph buildgraph") {
		t.Fatalf("expected graph header")
	}
	if !strings.Contains(dot, "\"v1\" -> \"v2\"") {
		t.Fatalf("expected edge in dot output: %s", dot)
	}
}

func TestRenderSVGReturnsErrorWhenDotMissing(t *testing.T) {
	t.Setenv("PATH", "")
	err := RenderSVG("digraph buildgraph {\n}\n", filepath.Join(t.TempDir(), "graph.svg"))
	if err == nil {
		t.Fatalf("expected render svg to fail without dot binary")
	}
}
