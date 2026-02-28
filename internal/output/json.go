package output

import (
	"encoding/json"
	"io"
	"time"
)

const APIVersion = "buildgraph.dev/v1"
const SchemaVersion = "1"

type ErrorItem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Envelope struct {
	APIVersion    string      `json:"apiVersion"`
	Command       string      `json:"command"`
	SchemaVersion string      `json:"schemaVersion"`
	Timestamp     time.Time   `json:"timestamp"`
	DurationMS    int64       `json:"durationMs"`
	Result        any         `json:"result"`
	Errors        []ErrorItem `json:"errors"`
}

func NewEnvelope(command string, startedAt time.Time, result any, errors []ErrorItem) Envelope {
	duration := time.Since(startedAt).Milliseconds()
	if duration < 0 {
		duration = 0
	}
	return NewEnvelopeWithDuration(command, duration, result, errors)
}

func NewEnvelopeWithDuration(command string, durationMS int64, result any, errors []ErrorItem) Envelope {
	if errors == nil {
		errors = []ErrorItem{}
	}
	return Envelope{
		APIVersion:    APIVersion,
		Command:       command,
		SchemaVersion: SchemaVersion,
		Timestamp:     time.Now().UTC(),
		DurationMS:    durationMS,
		Result:        result,
		Errors:        errors,
	}
}

func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
