package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type LoadOptions struct {
	CWD         string
	GlobalPath  string
	ProjectPath string
	Profile     string
	Override    Override
}

func Load(opts LoadOptions) (Loaded, error) {
	cfg := DefaultConfig()
	paths, err := resolvePaths(opts)
	if err != nil {
		return Loaded{}, err
	}

	if paths.GlobalExists {
		globalCfg, err := readConfig(paths.GlobalPath)
		if err != nil {
			return Loaded{}, fmt.Errorf("read global config: %w", err)
		}
		cfg = merge(cfg, globalCfg)
	}

	if paths.ProjectExists {
		projectCfg, err := readConfig(paths.ProjectPath)
		if err != nil {
			return Loaded{}, fmt.Errorf("read project config: %w", err)
		}
		cfg = merge(cfg, projectCfg)
	}

	cfg = applyEnv(cfg)
	cfg = applyProfile(cfg, opts.Profile)
	cfg = applyOverride(cfg, opts.Override)

	return Loaded{
		Config:   cfg,
		Paths:    paths,
		LoadedAt: time.Now().UTC(),
	}, nil
}

func resolvePaths(opts LoadOptions) (Paths, error) {
	cwd := opts.CWD
	if cwd == "" {
		wd, err := os.Getwd()
		if err != nil {
			return Paths{}, fmt.Errorf("resolve cwd: %w", err)
		}
		cwd = wd
	}

	globalPath := opts.GlobalPath
	if globalPath == "" {
		var err error
		globalPath, err = DefaultGlobalConfigPath()
		if err != nil {
			return Paths{}, err
		}
	}
	projectPath := opts.ProjectPath
	if projectPath == "" {
		projectPath = findProjectConfig(cwd)
		if projectPath == "" {
			projectPath = filepath.Join(cwd, ".buildgraph.yaml")
		}
	}

	globalExists := fileExists(globalPath)
	projectExists := fileExists(projectPath)

	return Paths{
		GlobalPath:    globalPath,
		ProjectPath:   projectPath,
		GlobalExists:  globalExists,
		ProjectExists: projectExists,
	}, nil
}

func DefaultGlobalConfigPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, "buildgraph", "config.yaml"), nil
}

func findProjectConfig(start string) string {
	current := start
	for {
		candidate := filepath.Join(current, ".buildgraph.yaml")
		if fileExists(candidate) {
			return candidate
		}
		next := filepath.Dir(current)
		if next == current {
			return ""
		}
		current = next
	}
}

func readConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]ProfileConfig{}
	}
	return cfg, nil
}

func merge(base, overlay Config) Config {
	result := base
	if overlay.Backend != "" {
		result.Backend = overlay.Backend
	}
	if overlay.Endpoint != "" {
		result.Endpoint = overlay.Endpoint
	}
	if overlay.Telemetry.Sink != "" {
		result.Telemetry.Sink = overlay.Telemetry.Sink
	}
	if overlay.Telemetry.Enabled {
		result.Telemetry.Enabled = true
	}
	if overlay.Auth.Endpoint != "" {
		result.Auth.Endpoint = overlay.Auth.Endpoint
	}
	if overlay.Auth.User != "" {
		result.Auth.User = overlay.Auth.User
	}
	if overlay.Defaults.Analyze.Dockerfile != "" {
		result.Defaults.Analyze.Dockerfile = overlay.Defaults.Analyze.Dockerfile
	}
	if overlay.Defaults.Analyze.SeverityThreshold != "" {
		result.Defaults.Analyze.SeverityThreshold = overlay.Defaults.Analyze.SeverityThreshold
	}
	if overlay.Defaults.Analyze.FailOn != "" {
		result.Defaults.Analyze.FailOn = overlay.Defaults.Analyze.FailOn
	}
	if overlay.Defaults.Build.Dockerfile != "" {
		result.Defaults.Build.Dockerfile = overlay.Defaults.Build.Dockerfile
	}
	if overlay.Defaults.Build.OutputMode != "" {
		result.Defaults.Build.OutputMode = overlay.Defaults.Build.OutputMode
	}
	if overlay.Defaults.Build.ImageRef != "" {
		result.Defaults.Build.ImageRef = overlay.Defaults.Build.ImageRef
	}
	if result.Profiles == nil {
		result.Profiles = map[string]ProfileConfig{}
	}
	for name, profile := range overlay.Profiles {
		result.Profiles[name] = profile
	}
	return result
}

func applyEnv(cfg Config) Config {
	if v := strings.TrimSpace(os.Getenv("BUILDGRAPH_BACKEND")); v != "" {
		cfg.Backend = v
	}
	if v := strings.TrimSpace(os.Getenv("BUILDKIT_HOST")); v != "" {
		cfg.Endpoint = v
	}
	if v := strings.TrimSpace(os.Getenv("BUILDGRAPH_ENDPOINT")); v != "" {
		cfg.Endpoint = v
	}
	if v := strings.TrimSpace(os.Getenv("BUILDGRAPH_TELEMETRY")); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.Telemetry.Enabled = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("BUILDGRAPH_AUTH_ENDPOINT")); v != "" {
		cfg.Auth.Endpoint = v
	}
	return cfg
}

func applyProfile(cfg Config, profileName string) Config {
	if profileName == "" {
		return cfg
	}
	if profile, ok := cfg.Profiles[profileName]; ok {
		if profile.Backend != "" {
			cfg.Backend = profile.Backend
		}
		if profile.Endpoint != "" {
			cfg.Endpoint = profile.Endpoint
		}
	}
	return cfg
}

func applyOverride(cfg Config, override Override) Config {
	if override.Backend != "" {
		cfg.Backend = override.Backend
	}
	if override.Endpoint != "" {
		cfg.Endpoint = override.Endpoint
	}
	return cfg
}

func EnsureParent(path string) error {
	if path == "" {
		return errors.New("path cannot be empty")
	}
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

func DefaultStateDBPath() (string, error) {
	if runtime.GOOS == "windows" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(base, "buildgraph", "state", "state.db"), nil
	}
	if xdgState := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); xdgState != "" {
		return filepath.Join(xdgState, "buildgraph", "state.db"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "buildgraph", "state.db"), nil
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
