package output

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestWriteJSONEnvelope(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	env := Envelope{
		APIVersion:    APIVersion,
		Command:       "analyze",
		SchemaVersion: SchemaVersion,
		Timestamp:     time.Unix(0, 0).UTC(),
		DurationMS:    12,
		Result: map[string]any{
			"ok": true,
		},
		Errors: []ErrorItem{},
	}
	if err := WriteJSON(buf, env); err != nil {
		t.Fatalf("write json: %v", err)
	}
	text := buf.String()
	if !strings.Contains(text, `"apiVersion": "buildgraph.dev/v1"`) {
		t.Fatalf("apiVersion missing: %s", text)
	}
	if !strings.Contains(text, `"schemaVersion": "1"`) {
		t.Fatalf("schemaVersion missing: %s", text)
	}
	if !strings.Contains(text, `"errors": []`) {
		t.Fatalf("errors array missing: %s", text)
	}
}
