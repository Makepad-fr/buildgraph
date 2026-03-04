package output

import (
	"encoding/json"
	"io"
	"time"
)

const APIVersion = "buildgraph.dev/v2"
const SchemaVersion = "2"

type ErrorItem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ResourceMetadata struct {
	Command     string    `json:"command"`
	GeneratedAt time.Time `json:"generatedAt"`
	RunID       int64     `json:"runId,omitempty"`
}

type ResourceStatus struct {
	Phase   string      `json:"phase"`
	Summary any         `json:"summary,omitempty"`
	Result  any         `json:"result,omitempty"`
	Errors  []ErrorItem `json:"errors,omitempty"`
}

type Resource struct {
	APIVersion string           `json:"apiVersion"`
	Kind       string           `json:"kind"`
	Metadata   ResourceMetadata `json:"metadata"`
	Spec       any              `json:"spec,omitempty"`
	Status     ResourceStatus   `json:"status"`
}

// Envelope is kept for compatibility with existing command helpers.
type Envelope struct {
	APIVersion    string      `json:"apiVersion"`
	Command       string      `json:"command"`
	SchemaVersion string      `json:"schemaVersion"`
	Timestamp     time.Time   `json:"timestamp"`
	DurationMS    int64       `json:"durationMs"`
	Result        any         `json:"result"`
	Errors        []ErrorItem `json:"errors"`
}

func SuccessResource(kind, command string, spec, summary, result any, runID int64) Resource {
	return Resource{
		APIVersion: APIVersion,
		Kind:       kind,
		Metadata: ResourceMetadata{
			Command:     command,
			GeneratedAt: time.Now().UTC(),
			RunID:       runID,
		},
		Spec: spec,
		Status: ResourceStatus{
			Phase:   "completed",
			Summary: summary,
			Result:  result,
		},
	}
}

func ErrorResource(kind, command string, spec, summary any, errs []ErrorItem, runID int64) Resource {
	return Resource{
		APIVersion: APIVersion,
		Kind:       kind,
		Metadata: ResourceMetadata{
			Command:     command,
			GeneratedAt: time.Now().UTC(),
			RunID:       runID,
		},
		Spec: spec,
		Status: ResourceStatus{
			Phase:   "failed",
			Summary: summary,
			Errors:  errs,
		},
	}
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
