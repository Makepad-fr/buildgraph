package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Makepad-fr/buildgraph/internal/output"
	"github.com/Makepad-fr/buildgraph/internal/trace"
)

func (a *App) runGraph(global GlobalOptions, args []string) (int, error) {
	startedAt := time.Now().UTC()

	fs := flag.NewFlagSet("graph", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	from := fs.String("from", "", "Trace JSONL path")
	format := fs.String("format", "dot", "Graph format: dot|svg|json")
	outputPath := fs.String("output", "", "Write output to file")

	if err := fs.Parse(args); err != nil {
		return ExitUsage, err
	}
	if strings.TrimSpace(*from) == "" {
		return ExitUsage, fmt.Errorf("--from is required")
	}

	renderFormat := strings.ToLower(strings.TrimSpace(*format))
	switch renderFormat {
	case "dot", "svg", "json":
	default:
		return ExitUsage, fmt.Errorf("unsupported graph format %q (expected dot|svg|json)", renderFormat)
	}
	if renderFormat == "svg" && strings.TrimSpace(*outputPath) == "" {
		return ExitUsage, fmt.Errorf("--output is required for --format=svg")
	}

	records, err := trace.LoadFile(*from)
	if err != nil {
		return ExitConfigState, err
	}
	graph, err := trace.BuildGraph(records)
	if err != nil {
		if errors.Is(err, trace.ErrNoVertexData) {
			return ExitBackend, err
		}
		return ExitInternal, err
	}

	content, err := renderGraph(graph, renderFormat)
	if err != nil {
		return ExitInternal, err
	}

	if path := strings.TrimSpace(*outputPath); path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return ExitConfigState, fmt.Errorf("create output dir: %w", err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return ExitConfigState, fmt.Errorf("write output file: %w", err)
		}
	}

	if global.JSON {
		result := map[string]any{
			"from":        *from,
			"format":      renderFormat,
			"vertexCount": len(graph.Vertices),
			"edgeCount":   len(graph.Edges),
			"outputPath":  strings.TrimSpace(*outputPath),
		}
		if strings.TrimSpace(*outputPath) == "" {
			switch renderFormat {
			case "json":
				result["graph"] = graph
			default:
				result["content"] = string(content)
			}
		}
		if err := output.WriteJSON(a.io.Out, output.NewEnvelope("graph", startedAt, result, nil)); err != nil {
			return ExitInternal, err
		}
		return ExitOK, nil
	}

	if path := strings.TrimSpace(*outputPath); path != "" {
		_, _ = fmt.Fprintf(a.io.Out, "Wrote graph output to %s\n", path)
		return ExitOK, nil
	}
	_, _ = a.io.Out.Write(content)
	if len(content) == 0 || content[len(content)-1] != '\n' {
		_, _ = fmt.Fprintln(a.io.Out)
	}
	return ExitOK, nil
}

func (a *App) runTop(global GlobalOptions, args []string) (int, error) {
	startedAt := time.Now().UTC()

	fs := flag.NewFlagSet("top", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	from := fs.String("from", "", "Trace JSONL path")
	limit := fs.Int("limit", 10, "Maximum rows for slowest vertices")

	if err := fs.Parse(args); err != nil {
		return ExitUsage, err
	}
	if strings.TrimSpace(*from) == "" {
		return ExitUsage, fmt.Errorf("--from is required")
	}
	if *limit <= 0 {
		return ExitUsage, fmt.Errorf("--limit must be greater than 0")
	}

	records, err := trace.LoadFile(*from)
	if err != nil {
		return ExitConfigState, err
	}
	graph, err := trace.BuildGraph(records)
	if err != nil {
		if errors.Is(err, trace.ErrNoVertexData) {
			return ExitBackend, err
		}
		return ExitInternal, err
	}
	result := trace.AnalyzeTop(graph, *limit)

	if global.JSON {
		payload := map[string]any{
			"from": *from,
			"top":  result,
		}
		if err := output.WriteJSON(a.io.Out, output.NewEnvelope("top", startedAt, payload, nil)); err != nil {
			return ExitInternal, err
		}
		return ExitOK, nil
	}

	if err := writeTopHuman(a.io.Out, result); err != nil {
		return ExitInternal, err
	}
	return ExitOK, nil
}

func renderGraph(graph trace.Graph, format string) ([]byte, error) {
	switch format {
	case "dot":
		return []byte(trace.DOT(graph)), nil
	case "json":
		payload, err := json.MarshalIndent(graph, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal graph json: %w", err)
		}
		return append(payload, '\n'), nil
	case "svg":
		return renderDOTAsSVG(trace.DOT(graph))
	default:
		return nil, fmt.Errorf("unsupported graph format %q", format)
	}
}

func renderDOTAsSVG(dotSource string) ([]byte, error) {
	cmd := exec.Command("dot", "-Tsvg")
	cmd.Stdin = strings.NewReader(dotSource)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("graphviz 'dot' is required for --format=svg. install Graphviz and retry, or use --format=dot")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("render svg with graphviz: %s", msg)
	}
	return stdout.Bytes(), nil
}

func writeTopHuman(w io.Writer, result trace.TopResult) error {
	if _, err := fmt.Fprintf(w, "Vertices analyzed: %d\n", result.VertexCount); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w, "\nSlowest vertices:"); err != nil {
		return err
	}
	if len(result.Slowest) == 0 {
		if _, err := fmt.Fprintln(w, "none"); err != nil {
			return err
		}
	} else {
		for i, row := range result.Slowest {
			if _, err := fmt.Fprintf(w, "%d. %d ms  %s (%s)\n", i+1, row.DurationMS, row.Name, row.ID); err != nil {
				return err
			}
		}
	}

	if _, err := fmt.Fprintf(w, "\nCritical path: %d ms\n", result.CriticalPath.DurationMS); err != nil {
		return err
	}
	if len(result.CriticalPath.Vertices) == 0 {
		if _, err := fmt.Fprintln(w, "none"); err != nil {
			return err
		}
	} else {
		for i, row := range result.CriticalPath.Vertices {
			if _, err := fmt.Fprintf(w, "%d. %s (%d ms)\n", i+1, row.Name, row.DurationMS); err != nil {
				return err
			}
		}
	}

	if result.HasCycle {
		if _, err := fmt.Fprintf(w, "\nWarning: %s\n", result.CycleDetected); err != nil {
			return err
		}
	}
	return nil
}
