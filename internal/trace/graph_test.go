package trace

import (
	"errors"
	"testing"
	"time"

	"github.com/Makepad-fr/buildgraph/internal/backend"
)

func TestBuildGraphMergesVerticesAndComputesTop(t *testing.T) {
	t.Parallel()

	t0 := time.Unix(0, 0).UTC()
	t5 := time.Unix(0, int64(5*time.Millisecond)).UTC()
	t6 := time.Unix(0, int64(6*time.Millisecond)).UTC()
	t15 := time.Unix(0, int64(15*time.Millisecond)).UTC()
	t20 := time.Unix(0, int64(20*time.Millisecond)).UTC()

	records := []Record{
		ProgressRecord("build", backend.BuildProgressEvent{
			VertexID:  "a",
			Message:   "FROM alpine",
			Started:   &t0,
			Completed: &t5,
		}),
		ProgressRecord("build", backend.BuildProgressEvent{
			VertexID:  "b",
			Message:   "RUN apk add curl",
			Inputs:    []string{"a"},
			Started:   &t5,
			Completed: &t20,
		}),
		ProgressRecord("build", backend.BuildProgressEvent{
			VertexID:  "c",
			Message:   "RUN echo hi",
			Inputs:    []string{"a"},
			Started:   &t6,
			Completed: &t15,
		}),
		ProgressRecord("build", backend.BuildProgressEvent{
			VertexID: "b",
			Cached:   true,
		}),
	}

	graph, err := BuildGraph(records)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}
	if got, want := len(graph.Vertices), 3; got != want {
		t.Fatalf("unexpected vertex count: got=%d want=%d", got, want)
	}
	if got, want := len(graph.Edges), 2; got != want {
		t.Fatalf("unexpected edge count: got=%d want=%d", got, want)
	}

	var vertexB Vertex
	for _, vertex := range graph.Vertices {
		if vertex.ID == "b" {
			vertexB = vertex
			break
		}
	}
	if got, want := vertexB.DurationMS, int64(15); got != want {
		t.Fatalf("unexpected vertex b duration: got=%d want=%d", got, want)
	}
	if !vertexB.Cached {
		t.Fatalf("expected merged cache=true for vertex b")
	}

	top := AnalyzeTop(graph, 2)
	if got, want := len(top.Slowest), 2; got != want {
		t.Fatalf("unexpected slowest count: got=%d want=%d", got, want)
	}
	if got, want := top.Slowest[0].ID, "b"; got != want {
		t.Fatalf("unexpected slowest[0]: got=%q want=%q", got, want)
	}
	if got, want := top.CriticalPath.DurationMS, int64(20); got != want {
		t.Fatalf("unexpected critical path duration: got=%d want=%d", got, want)
	}
	if got, want := len(top.CriticalPath.Vertices), 2; got != want {
		t.Fatalf("unexpected critical path length: got=%d want=%d", got, want)
	}
	if got, want := top.CriticalPath.Vertices[0].ID, "a"; got != want {
		t.Fatalf("unexpected critical path first vertex: got=%q want=%q", got, want)
	}
	if got, want := top.CriticalPath.Vertices[1].ID, "b"; got != want {
		t.Fatalf("unexpected critical path second vertex: got=%q want=%q", got, want)
	}
}

func TestBuildGraphRequiresVertexIDs(t *testing.T) {
	t.Parallel()

	_, err := BuildGraph([]Record{
		ProgressRecord("build", backend.BuildProgressEvent{Message: "plain log"}),
	})
	if !errors.Is(err, ErrNoVertexData) {
		t.Fatalf("expected ErrNoVertexData, got %v", err)
	}
}
