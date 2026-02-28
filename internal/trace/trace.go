package trace

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Makepad-fr/buildgraph/internal/backend"
)

const SchemaVersion = "1"

const (
	KindProgress = "progress"
	KindResult   = "result"
	KindError    = "error"
)

type ErrorRecord struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Record struct {
	SchemaVersion string                      `json:"schemaVersion"`
	Kind          string                      `json:"kind"`
	Command       string                      `json:"command"`
	Timestamp     time.Time                   `json:"timestamp"`
	Progress      *backend.BuildProgressEvent `json:"progress,omitempty"`
	Result        *backend.BuildResult        `json:"result,omitempty"`
	Error         *ErrorRecord                `json:"error,omitempty"`
}

type Writer struct {
	mu  sync.Mutex
	enc *json.Encoder
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{
		enc: json.NewEncoder(w),
	}
}

func (w *Writer) WriteRecord(rec Record) error {
	if w == nil || w.enc == nil {
		return fmt.Errorf("trace writer is not initialized")
	}
	rec = normalizedRecord(rec)
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.enc.Encode(rec)
}

func OpenFileWriter(path string) (*os.File, *Writer, error) {
	if path == "" {
		return nil, nil, fmt.Errorf("trace path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, fmt.Errorf("create trace parent dir: %w", err)
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("create trace file: %w", err)
	}
	return file, NewWriter(file), nil
}

func ReadRecords(r io.Reader) ([]Record, error) {
	dec := json.NewDecoder(bufio.NewReader(r))
	records := make([]Record, 0)
	for {
		var rec Record
		err := dec.Decode(&rec)
		if errors.Is(err, io.EOF) {
			return records, nil
		}
		if err != nil {
			return nil, fmt.Errorf("decode trace record: %w", err)
		}
		records = append(records, normalizedRecord(rec))
	}
}

func LoadFile(path string) ([]Record, error) {
	if path == "" {
		return nil, fmt.Errorf("trace path is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open trace file: %w", err)
	}
	defer file.Close()
	return ReadRecords(file)
}

func ProgressRecord(command string, event backend.BuildProgressEvent) Record {
	cloned := event
	return Record{
		Kind:      KindProgress,
		Command:   command,
		Timestamp: time.Now().UTC(),
		Progress:  &cloned,
	}
}

func ResultRecord(command string, result backend.BuildResult) Record {
	cloned := result
	return Record{
		Kind:      KindResult,
		Command:   command,
		Timestamp: time.Now().UTC(),
		Result:    &cloned,
	}
}

func FailureRecord(command, code, message string) Record {
	return Record{
		Kind:      KindError,
		Command:   command,
		Timestamp: time.Now().UTC(),
		Error: &ErrorRecord{
			Code:    code,
			Message: message,
		},
	}
}

func normalizedRecord(rec Record) Record {
	if rec.SchemaVersion == "" {
		rec.SchemaVersion = SchemaVersion
	}
	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now().UTC()
	}
	return rec
}
