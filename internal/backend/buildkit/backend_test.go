package buildkit

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Makepad-fr/buildgraph/internal/backend"
)

type fakeDirect struct {
	ping map[string]error
}

func (f *fakeDirect) Ping(_ context.Context, endpoint string) error {
	if f.ping == nil {
		return fmt.Errorf("ping failed: %s", endpoint)
	}
	if err, ok := f.ping[endpoint]; ok {
		return err
	}
	return fmt.Errorf("ping failed: %s", endpoint)
}

func (f *fakeDirect) Build(context.Context, string, backend.BuildRequest, backend.BuildProgressFunc) (backend.BuildResult, error) {
	return backend.BuildResult{}, nil
}

type fakeDocker struct {
	pingErr error
}

func (f *fakeDocker) Ping(context.Context) error {
	if f.pingErr != nil {
		return f.pingErr
	}
	return nil
}

func (f *fakeDocker) Build(context.Context, backend.BuildRequest, backend.BuildProgressFunc) (backend.BuildResult, error) {
	return backend.BuildResult{}, nil
}

func TestResolveEndpointRecordsAttemptsInPriorityOrder(t *testing.T) {
	t.Setenv("BUILDKIT_HOST", "unix:///env.sock")

	be := &Backend{
		direct: &fakeDirect{
			ping: map[string]error{
				"unix:///flag.sock": errors.New("flag unavailable"),
				"unix:///env.sock":  nil,
			},
		},
		docker: &fakeDocker{pingErr: errors.New("docker unavailable")},
	}

	resolved, err := be.resolveEndpoint(context.Background(), "unix:///flag.sock", "", "")
	if err != nil {
		t.Fatalf("resolve endpoint: %v", err)
	}
	if got, want := resolved.Source, "env"; got != want {
		t.Fatalf("unexpected source: got=%q want=%q", got, want)
	}
	if got, want := resolved.Endpoint, "unix:///env.sock"; got != want {
		t.Fatalf("unexpected endpoint: got=%q want=%q", got, want)
	}
	if len(resolved.Attempts) < 2 {
		t.Fatalf("expected at least 2 attempts, got %d", len(resolved.Attempts))
	}
	if got, want := resolved.Attempts[0].Source, "flag"; got != want {
		t.Fatalf("unexpected first source: got=%q want=%q", got, want)
	}
	if got, want := resolved.Attempts[1].Source, "env"; got != want {
		t.Fatalf("unexpected second source: got=%q want=%q", got, want)
	}
	if got, want := resolved.Attempts[0].Status, "error"; got != want {
		t.Fatalf("unexpected flag attempt status: got=%q want=%q", got, want)
	}
	if got, want := resolved.Attempts[1].Status, "ok"; got != want {
		t.Fatalf("unexpected env attempt status: got=%q want=%q", got, want)
	}
}

func TestDetectReturnsAttemptTrailOnFailure(t *testing.T) {
	be := &Backend{
		direct: &fakeDirect{
			ping: map[string]error{
				"unix:///flag.sock": errors.New("permission denied"),
			},
		},
		docker: &fakeDocker{pingErr: errors.New("docker unavailable")},
	}

	result, err := be.Detect(context.Background(), backend.DetectRequest{
		Endpoint: "unix:///flag.sock",
	})
	if err == nil {
		t.Fatalf("expected detect error")
	}
	if result.Available {
		t.Fatalf("expected detect availability to be false")
	}
	if len(result.Attempts) == 0 {
		t.Fatalf("expected detect attempts to be recorded")
	}
	if got, want := result.Attempts[0].Source, "flag"; got != want {
		t.Fatalf("unexpected first attempt source: got=%q want=%q", got, want)
	}
	if got, want := result.Attempts[0].Status, "error"; got != want {
		t.Fatalf("unexpected first attempt status: got=%q want=%q", got, want)
	}
}
