package report

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/Makepad-fr/buildgraph/internal/backend"
)

func RenderDOT(run BuildReport) string {
	criticalSet := map[string]bool{}
	for _, id := range run.Metrics.CriticalPathVertices {
		criticalSet[id] = true
	}

	var b strings.Builder
	b.WriteString("digraph buildgraph {\n")
	b.WriteString("  rankdir=LR;\n")
	b.WriteString("  node [shape=box style=filled fillcolor=\"#f8f8f8\"];\n")

	vertices := append([]backend.BuildVertex(nil), run.Build.Vertices...)
	sort.Slice(vertices, func(i, j int) bool {
		return vertices[i].ID < vertices[j].ID
	})
	for _, vertex := range vertices {
		color := "#f8f8f8"
		if vertex.Cached {
			color = "#d9f6dc"
		} else {
			color = "#ffd9d9"
		}
		if criticalSet[vertex.ID] {
			color = "#ffec99"
		}
		label := vertex.Name
		if label == "" {
			label = vertex.ID
		}
		label = strings.ReplaceAll(label, "\"", "'")
		fmt.Fprintf(&b, "  \"%s\" [label=\"%s\\n%dms\" fillcolor=\"%s\"];\n", vertex.ID, label, vertex.DurationMS, color)
	}

	edges := append([]backend.BuildEdge(nil), run.Build.Edges...)
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From == edges[j].From {
			return edges[i].To < edges[j].To
		}
		return edges[i].From < edges[j].From
	})
	for _, edge := range edges {
		attrs := ""
		if criticalSet[edge.From] && criticalSet[edge.To] {
			attrs = " [color=\"#d9480f\" penwidth=2]"
		}
		fmt.Fprintf(&b, "  \"%s\" -> \"%s\"%s;\n", edge.From, edge.To, attrs)
	}

	b.WriteString("}\n")
	return b.String()
}

func WriteDOTFile(path string, run BuildReport) error {
	return os.WriteFile(path, []byte(RenderDOT(run)), 0o644)
}

func RenderSVG(dot, outPath string) error {
	cmd := exec.Command("dot", "-Tsvg", "-o", outPath)
	cmd.Stdin = strings.NewReader(dot)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("graphviz dot failed: %s", strings.TrimSpace(stderr.String()))
		}
		return fmt.Errorf("graphviz dot failed: %w", err)
	}
	return nil
}

func GraphvizVersion() (string, error) {
	cmd := exec.Command("dot", "-V")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
