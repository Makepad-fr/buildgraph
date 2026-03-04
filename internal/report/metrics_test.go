package report

import (
	"testing"

	"github.com/Makepad-fr/buildgraph/internal/backend"
)

func TestComputeBuildMetricsCriticalPath(t *testing.T) {
	t.Parallel()
	vertices := []backend.BuildVertex{
		{ID: "A", Name: "A", DurationMS: 100, Cached: false, Stage: "build"},
		{ID: "B", Name: "B", DurationMS: 50, Cached: true, Stage: "build"},
		{ID: "C", Name: "C", DurationMS: 25, Cached: false, Stage: "runtime"},
	}
	edges := []backend.BuildEdge{{From: "A", To: "B"}, {From: "B", To: "C"}}
	metrics := ComputeBuildMetrics(vertices, edges, backend.CacheStats{Hits: 1, Misses: 2})

	if metrics.CriticalPathMS != 175 {
		t.Fatalf("expected critical path 175ms, got %d", metrics.CriticalPathMS)
	}
	if metrics.LongestChain != 3 {
		t.Fatalf("expected longest chain 3, got %d", metrics.LongestChain)
	}
	if len(metrics.CriticalPathVertices) != 3 || metrics.CriticalPathVertices[0] != "A" || metrics.CriticalPathVertices[2] != "C" {
		t.Fatalf("unexpected critical path vertices: %v", metrics.CriticalPathVertices)
	}
	if metrics.CacheHitRatio != (1.0 / 3.0) {
		t.Fatalf("unexpected cache hit ratio: %f", metrics.CacheHitRatio)
	}
	if metrics.StageDistribution["build"] != 150 {
		t.Fatalf("unexpected build stage distribution: %d", metrics.StageDistribution["build"])
	}
	if metrics.TimeDistribution["totalMs"] != 175 {
		t.Fatalf("unexpected total duration: %d", metrics.TimeDistribution["totalMs"])
	}
}
