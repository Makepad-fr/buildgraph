package analyze

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseStageGraphMultiStage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")
	content := `FROM golang:1.22 AS builder
WORKDIR /src
RUN go build ./...

FROM alpine:3.20 AS runtime
COPY --from=builder /src/app /app
ENTRYPOINT ["/app"]
`
	if err := os.WriteFile(dockerfile, []byte(content), 0o644); err != nil {
		t.Fatalf("write dockerfile: %v", err)
	}

	graph, err := ParseStageGraph(dir, "Dockerfile")
	if err != nil {
		t.Fatalf("parse stage graph: %v", err)
	}
	if len(graph.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(graph.Stages))
	}
	if graph.Stages[0].Name != "builder" {
		t.Fatalf("expected first stage builder, got %q", graph.Stages[0].Name)
	}
	if graph.Stages[1].Name != "runtime" {
		t.Fatalf("expected second stage runtime, got %q", graph.Stages[1].Name)
	}
	if len(graph.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(graph.Edges))
	}
	edge := graph.Edges[0]
	if edge.From != "builder" || edge.To != "runtime" || edge.Reason != "copy-from" {
		t.Fatalf("unexpected edge: %+v", edge)
	}
}
