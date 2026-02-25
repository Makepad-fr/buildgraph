package output

import (
	"encoding/json"
	"io"
	"time"
)

const APIVersion = "buildgraph.dev/v1"

type ErrorItem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Envelope struct {
	APIVersion string      `json:"apiVersion"`
	Command    string      `json:"command"`
	Timestamp  time.Time   `json:"timestamp"`
	DurationMS int64       `json:"durationMs"`
	Result     any         `json:"result"`
	Errors     []ErrorItem `json:"errors,omitempty"`
}

func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
