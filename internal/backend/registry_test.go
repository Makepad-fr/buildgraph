package backend

import (
	"context"
	"testing"
)

type fakeBackend struct {
	name string
}

func (f fakeBackend) Name() string { return f.name }
func (f fakeBackend) Detect(context.Context, DetectRequest) (DetectResult, error) {
	return DetectResult{}, nil
}
func (f fakeBackend) Analyze(context.Context, AnalyzeRequest) (AnalyzeResult, error) {
	return AnalyzeResult{}, nil
}
func (f fakeBackend) Build(context.Context, BuildRequest, BuildProgressFunc) (BuildResult, error) {
	return BuildResult{}, nil
}
func (f fakeBackend) Capabilities(context.Context) (BackendCapabilities, error) {
	return BackendCapabilities{}, nil
}

func TestRegistryRegisterList(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	if err := r.Register(fakeBackend{name: "buildkit"}); err != nil {
		t.Fatalf("register backend: %v", err)
	}
	if _, ok := r.Get("buildkit"); !ok {
		t.Fatalf("backend was not found")
	}
	if len(r.List()) != 1 {
		t.Fatalf("expected one backend")
	}
}
