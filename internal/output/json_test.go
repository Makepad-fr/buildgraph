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
	resource := Resource{
		APIVersion: APIVersion,
		Kind:       "AnalyzeReport",
		Metadata: ResourceMetadata{
			Command:     "analyze",
			GeneratedAt: time.Unix(0, 0).UTC(),
		},
		Status: ResourceStatus{
			Phase: "completed",
			Result: map[string]any{
				"ok": true,
			},
		},
	}
	if err := WriteJSON(buf, resource); err != nil {
		t.Fatalf("write json: %v", err)
	}
	text := buf.String()
	if !strings.Contains(text, `"apiVersion": "buildgraph.dev/v2"`) {
		t.Fatalf("apiVersion missing: %s", text)
	}
	if !strings.Contains(text, `"kind": "AnalyzeReport"`) {
		t.Fatalf("kind missing: %s", text)
	}
}
