package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Makepad-fr/buildgraph/internal/backend"
	"github.com/Makepad-fr/buildgraph/internal/config"
	"github.com/Makepad-fr/buildgraph/internal/output"
	"github.com/Makepad-fr/buildgraph/internal/trace"
)

func TestRunGraphDotAndJSON(t *testing.T) {
	t.Parallel()

	tracePath := writeTraceFixture(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app, err := NewApp(IO{In: strings.NewReader(""), Out: stdout, Err: stderr})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}

	code, err := app.runGraph(GlobalOptions{}, []string{"--from", tracePath, "--format", "dot"})
	if err != nil {
		t.Fatalf("run graph dot: %v", err)
	}
	if code != ExitOK {
		t.Fatalf("unexpected exit code: %d", code)
	}
	if !strings.Contains(stdout.String(), "digraph buildgraph") {
		t.Fatalf("dot output missing graph header: %s", stdout.String())
	}

	stdout.Reset()
	code, err = app.runGraph(GlobalOptions{}, []string{"--from", tracePath, "--format", "json"})
	if err != nil {
		t.Fatalf("run graph json: %v", err)
	}
	if code != ExitOK {
		t.Fatalf("unexpected exit code: %d", code)
	}

	var graph trace.Graph
	if err := json.Unmarshal(stdout.Bytes(), &graph); err != nil {
		t.Fatalf("decode graph json: %v", err)
	}
	if len(graph.Vertices) == 0 {
		t.Fatalf("expected graph vertices in json output")
	}
}

func TestRunTopPrintsCriticalPath(t *testing.T) {
	t.Parallel()

	tracePath := writeTraceFixture(t)
	stdout := &bytes.Buffer{}
	app, err := NewApp(IO{In: strings.NewReader(""), Out: stdout, Err: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}

	code, err := app.runTop(GlobalOptions{}, []string{"--from", tracePath, "--limit", "2"})
	if err != nil {
		t.Fatalf("run top: %v", err)
	}
	if code != ExitOK {
		t.Fatalf("unexpected exit code: %d", code)
	}
	text := stdout.String()
	if !strings.Contains(text, "Slowest vertices:") {
		t.Fatalf("top output missing slowest section: %s", text)
	}
	if !strings.Contains(text, "Critical path:") {
		t.Fatalf("top output missing critical path section: %s", text)
	}
}

func TestRunGraphSVGRequiresGraphviz(t *testing.T) {
	tracePath := writeTraceFixture(t)
	t.Setenv("PATH", t.TempDir())

	app, err := NewApp(IO{In: strings.NewReader(""), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}

	code, err := app.runGraph(GlobalOptions{}, []string{
		"--from", tracePath,
		"--format", "svg",
		"--output", filepath.Join(t.TempDir(), "graph.svg"),
	})
	if err == nil {
		t.Fatalf("expected graphviz error")
	}
	if code != ExitInternal {
		t.Fatalf("unexpected exit code: %d", code)
	}
	if !strings.Contains(err.Error(), "graphviz 'dot' is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunBuildRejectsInvalidProgressMode(t *testing.T) {
	t.Parallel()

	app, err := NewApp(IO{In: strings.NewReader(""), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}

	loaded := config.Loaded{
		Config: config.DefaultConfig(),
	}
	_, code, err := app.runBuild(context.Background(), GlobalOptions{}, loaded, []string{"--progress", "invalid"})
	if err == nil {
		t.Fatalf("expected invalid progress error")
	}
	if code != ExitUsage {
		t.Fatalf("unexpected exit code: %d", code)
	}
}

func TestRunDoctorHumanOutputHasRemediationSections(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	app, err := NewApp(IO{In: strings.NewReader(""), Out: stdout, Err: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}

	loaded := config.Loaded{
		Config: config.Config{
			Backend:  "missing-backend",
			Endpoint: "",
		},
		Paths: config.Paths{
			GlobalPath:  "/tmp/missing-global.yaml",
			ProjectPath: "/tmp/missing-project.yaml",
		},
	}
	code, err := app.runDoctor(context.Background(), GlobalOptions{}, loaded, nil)
	if err == nil {
		t.Fatalf("expected doctor error")
	}
	if code != ExitBackend {
		t.Fatalf("unexpected exit code: %d", code)
	}
	text := stdout.String()
	if !strings.Contains(text, "What it tried:") {
		t.Fatalf("doctor output missing tried section: %s", text)
	}
	if !strings.Contains(text, "What it found:") {
		t.Fatalf("doctor output missing found section: %s", text)
	}
	if !strings.Contains(text, "Paste into .buildgraph.yaml:") {
		t.Fatalf("doctor output missing config snippet section: %s", text)
	}
	if !strings.Contains(text, "Common fixes:") {
		t.Fatalf("doctor output missing fixes section: %s", text)
	}
}

func TestRunTopJSONEnvelopeIncludesSchemaVersion(t *testing.T) {
	t.Parallel()

	tracePath := writeTraceFixture(t)
	stdout := &bytes.Buffer{}
	app, err := NewApp(IO{In: strings.NewReader(""), Out: stdout, Err: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}

	code, err := app.runTop(GlobalOptions{JSON: true}, []string{"--from", tracePath, "--limit", "2"})
	if err != nil {
		t.Fatalf("run top json: %v", err)
	}
	if code != ExitOK {
		t.Fatalf("unexpected exit code: %d", code)
	}

	var env output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if got, want := env.SchemaVersion, output.SchemaVersion; got != want {
		t.Fatalf("unexpected schema version: got=%q want=%q", got, want)
	}
}

func writeTraceFixture(t *testing.T) string {
	t.Helper()

	file, writer, err := trace.OpenFileWriter(filepath.Join(t.TempDir(), "trace.jsonl"))
	if err != nil {
		t.Fatalf("open trace writer: %v", err)
	}
	defer file.Close()

	t0 := time.Unix(0, 0).UTC()
	t5 := time.Unix(0, int64(5*time.Millisecond)).UTC()
	t20 := time.Unix(0, int64(20*time.Millisecond)).UTC()

	if err := writer.WriteRecord(trace.ProgressRecord("build", backend.BuildProgressEvent{
		VertexID:  "a",
		Message:   "FROM alpine",
		Started:   &t0,
		Completed: &t5,
	})); err != nil {
		t.Fatalf("write trace record a: %v", err)
	}
	if err := writer.WriteRecord(trace.ProgressRecord("build", backend.BuildProgressEvent{
		VertexID:  "b",
		Message:   "RUN apk add curl",
		Inputs:    []string{"a"},
		Started:   &t5,
		Completed: &t20,
	})); err != nil {
		t.Fatalf("write trace record b: %v", err)
	}
	return file.Name()
}
