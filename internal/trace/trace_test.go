package trace

import (
	"bytes"
	"testing"
	"time"

	"github.com/Makepad-fr/buildgraph/internal/backend"
)

func TestTraceRoundTripNDJSON(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	writer := NewWriter(buf)
	if err := writer.WriteRecord(ProgressRecord("build", backend.BuildProgressEvent{
		Timestamp: time.Unix(1, 0).UTC(),
		Phase:     "build",
		Message:   "step",
		VertexID:  "v1",
		Status:    "running",
	})); err != nil {
		t.Fatalf("write progress: %v", err)
	}
	if err := writer.WriteRecord(ResultRecord("build", backend.BuildResult{Digest: "sha256:test"})); err != nil {
		t.Fatalf("write result: %v", err)
	}
	if err := writer.WriteRecord(FailureRecord("build", "build_failed", "boom")); err != nil {
		t.Fatalf("write error: %v", err)
	}

	records, err := ReadRecords(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("read records: %v", err)
	}
	if got, want := len(records), 3; got != want {
		t.Fatalf("unexpected record count: got=%d want=%d", got, want)
	}
	if got, want := records[0].SchemaVersion, SchemaVersion; got != want {
		t.Fatalf("unexpected schema version: got=%q want=%q", got, want)
	}
	if got, want := records[0].Kind, KindProgress; got != want {
		t.Fatalf("unexpected first kind: got=%q want=%q", got, want)
	}
	if got, want := records[1].Kind, KindResult; got != want {
		t.Fatalf("unexpected second kind: got=%q want=%q", got, want)
	}
	if got, want := records[2].Kind, KindError; got != want {
		t.Fatalf("unexpected third kind: got=%q want=%q", got, want)
	}
	if records[2].Error == nil || records[2].Error.Message != "boom" {
		t.Fatalf("unexpected error payload: %+v", records[2].Error)
	}
}
