package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigPrecedence(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yaml")
	projectPath := filepath.Join(dir, ".buildgraph.yaml")

	globalConfig := []byte("backend: buildkit\nendpoint: unix:///global.sock\n")
	projectConfig := []byte("endpoint: unix:///project.sock\n")
	if err := os.WriteFile(globalPath, globalConfig, 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	if err := os.WriteFile(projectPath, projectConfig, 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	t.Setenv("BUILDGRAPH_BACKEND", "auto")
	t.Setenv("BUILDKIT_HOST", "unix:///env.sock")

	loaded, err := Load(LoadOptions{
		CWD:         dir,
		GlobalPath:  globalPath,
		ProjectPath: projectPath,
		Override: Override{
			Endpoint: "unix:///flag.sock",
		},
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got, want := loaded.Config.Endpoint, "unix:///flag.sock"; got != want {
		t.Fatalf("unexpected endpoint: got=%q want=%q", got, want)
	}
	if got, want := loaded.Config.Backend, "auto"; got != want {
		t.Fatalf("unexpected backend: got=%q want=%q", got, want)
	}
}
