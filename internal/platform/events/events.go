package events

import (
	"context"
	"time"
)

type Event struct {
	RunID     int64     `json:"runId,omitempty"`
	Name      string    `json:"name"`
	Payload   any       `json:"payload,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type Sink interface {
	Emit(ctx context.Context, event Event) error
}

type Recorder interface {
	RecordEvent(ctx context.Context, runID int64, name string, payload any) error
}

type NoopSink struct{}

func (NoopSink) Emit(_ context.Context, _ Event) error {
	return nil
}

type LocalSink struct {
	Recorder Recorder
}

func (s LocalSink) Emit(ctx context.Context, event Event) error {
	if s.Recorder == nil {
		return nil
	}
	return s.Recorder.RecordEvent(ctx, event.RunID, event.Name, event.Payload)
}

type MultiSink struct {
	Sinks []Sink
}

func (s MultiSink) Emit(ctx context.Context, event Event) error {
	for _, sink := range s.Sinks {
		if sink == nil {
			continue
		}
		if err := sink.Emit(ctx, event); err != nil {
			return err
		}
	}
	return nil
}
