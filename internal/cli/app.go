package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Makepad-fr/buildgraph/internal/backend"
	"github.com/Makepad-fr/buildgraph/internal/backend/buildkit"
	"github.com/Makepad-fr/buildgraph/internal/config"
	"github.com/Makepad-fr/buildgraph/internal/output"
	"github.com/Makepad-fr/buildgraph/internal/platform/auth"
	"github.com/Makepad-fr/buildgraph/internal/platform/capabilities"
	"github.com/Makepad-fr/buildgraph/internal/platform/events"
	"github.com/Makepad-fr/buildgraph/internal/state"
	"github.com/Makepad-fr/buildgraph/internal/version"
)

const (
	ExitOK              = 0
	ExitUsage           = 2
	ExitConfigState     = 3
	ExitBackend         = 4
	ExitPolicyViolation = 5
	ExitAuthDenied      = 6
	ExitBuildFailed     = 7
	ExitInternal        = 10
)

type App struct {
	io           IO
	registry     *backend.Registry
	capabilities capabilities.Provider
}

type GlobalOptions struct {
	JSON           bool
	NoColor        bool
	Verbose        bool
	ConfigPath     string
	ProjectConfig  string
	Profile        string
	NonInteractive bool
}

func NewApp(io IO) (*App, error) {
	registry := backend.NewRegistry()
	if err := registry.Register(buildkit.NewBackend()); err != nil {
		return nil, err
	}
	return &App{
		io:           io,
		registry:     registry,
		capabilities: capabilities.NewAllAccessProvider(),
	}, nil
}

func (a *App) Run(ctx context.Context, args []string) int {
	global, remaining, err := parseGlobalFlags(args, a.io.Err)
	if err != nil {
		fmt.Fprintf(a.io.Err, "Error: %v\n", err)
		return ExitUsage
	}

	if len(remaining) == 0 {
		a.printHelp(a.io.Out)
		return ExitOK
	}

	loadedCfg, err := config.Load(config.LoadOptions{
		GlobalPath:  global.ConfigPath,
		ProjectPath: global.ProjectConfig,
		Profile:     global.Profile,
	})
	if err != nil {
		a.writeError(global.JSON, remaining[0], 0, err)
		return ExitConfigState
	}

	stateStore, stateErr := a.openStateStore()
	if stateErr == nil {
		defer stateStore.Close()
	}

	command := remaining[0]
	cmdArgs := remaining[1:]
	start := time.Now().UTC()
	exitCode := ExitOK
	runErr := error(nil)
	var recordFindings []backend.Finding
	var recordBuild *backend.BuildResult

	var emit events.Sink = events.NoopSink{}
	if stateStore != nil {
		emit = events.MultiSink{Sinks: []events.Sink{events.NoopSink{}, events.LocalSink{Recorder: stateStore}}}
	}

	switch command {
	case "help", "--help", "-h":
		a.printHelp(a.io.Out)
	case "analyze":
		result, findings, code, err := a.runAnalyze(ctx, global, loadedCfg, cmdArgs)
		exitCode = code
		runErr = err
		recordFindings = findings
		if err == nil {
			_ = emit.Emit(ctx, events.Event{Name: "analyze.completed", Payload: result, CreatedAt: time.Now().UTC()})
		}
	case "build":
		result, code, err := a.runBuild(ctx, global, loadedCfg, cmdArgs)
		exitCode = code
		runErr = err
		recordBuild = result
		if err == nil && result != nil {
			_ = emit.Emit(ctx, events.Event{Name: "build.completed", Payload: result, CreatedAt: time.Now().UTC()})
		}
	case "backend":
		exitCode, runErr = a.runBackend(global, cmdArgs)
	case "doctor":
		exitCode, runErr = a.runDoctor(ctx, global, loadedCfg, stateStore)
	case "auth":
		exitCode, runErr = a.runAuth(global, cmdArgs)
	case "config":
		exitCode, runErr = a.runConfig(global, loadedCfg, cmdArgs)
	case "version":
		exitCode, runErr = a.runVersion(global)
	default:
		runErr = fmt.Errorf("unknown command %q", command)
		exitCode = ExitUsage
		a.printHelp(a.io.Err)
	}

	if stateStore != nil {
		runID, err := stateStore.RecordRun(ctx, state.RunRecord{
			Command:    strings.Join(remaining, " "),
			StartedAt:  start,
			DurationMS: time.Since(start).Milliseconds(),
			Success:    runErr == nil,
			ExitCode:   exitCode,
			ErrorText:  errString(runErr),
		})
		if err == nil {
			if len(recordFindings) > 0 {
				_ = stateStore.RecordFindings(ctx, runID, recordFindings)
			}
			if recordBuild != nil {
				_ = stateStore.RecordBuild(ctx, runID, *recordBuild)
			}
		}
	}

	if runErr != nil {
		if !isReportedError(runErr) {
			a.writeError(global.JSON, command, time.Since(start).Milliseconds(), runErr)
		}
	}

	return exitCode
}

func (a *App) runAnalyze(ctx context.Context, global GlobalOptions, loaded config.Loaded, args []string) (backend.AnalyzeResult, []backend.Finding, int, error) {
	allowed, err := a.capabilities.Has(ctx, capabilities.FeatureAnalyze)
	if err != nil {
		return backend.AnalyzeResult{}, nil, ExitAuthDenied, err
	}
	if !allowed {
		return backend.AnalyzeResult{}, nil, ExitAuthDenied, fmt.Errorf("capability denied: analyze")
	}

	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	contextDir := fs.String("context", ".", "Build context path")
	dockerfile := fs.String("file", loaded.Config.Defaults.Analyze.Dockerfile, "Dockerfile path")
	severityThreshold := fs.String("severity-threshold", loaded.Config.Defaults.Analyze.SeverityThreshold, "Minimum severity: low|medium|high|critical")
	failOn := fs.String("fail-on", loaded.Config.Defaults.Analyze.FailOn, "Failure mode: policy|security|any")
	backendName := fs.String("backend", loaded.Config.Backend, "Backend selector")
	endpoint := fs.String("endpoint", loaded.Config.Endpoint, "BuildKit endpoint")

	if err := fs.Parse(args); err != nil {
		return backend.AnalyzeResult{}, nil, ExitUsage, err
	}

	selectedBackend, err := a.resolveBackend(*backendName)
	if err != nil {
		return backend.AnalyzeResult{}, nil, ExitBackend, err
	}

	result, err := selectedBackend.Analyze(ctx, backend.AnalyzeRequest{
		ContextDir:         *contextDir,
		Dockerfile:         *dockerfile,
		SeverityThreshold:  normalizeSeverity(*severityThreshold),
		FailOn:             strings.ToLower(strings.TrimSpace(*failOn)),
		Backend:            *backendName,
		Endpoint:           *endpoint,
		ProjectConfigPath:  loaded.Paths.ProjectPath,
		GlobalConfigPath:   loaded.Paths.GlobalPath,
		EnablePolicyChecks: true,
	})
	if err != nil {
		return backend.AnalyzeResult{}, nil, ExitBackend, err
	}

	failure := shouldFailFindings(result.Findings, strings.ToLower(strings.TrimSpace(*failOn)))
	failErr := fmt.Errorf("analysis found violations matching fail-on=%s", *failOn)

	if global.JSON {
		env := output.Envelope{
			APIVersion: output.APIVersion,
			Command:    "analyze",
			Timestamp:  time.Now().UTC(),
			DurationMS: 0,
			Result:     result,
		}
		if failure {
			env.Errors = []output.ErrorItem{{Code: "violation", Message: failErr.Error()}}
		}
		_ = output.WriteJSON(a.io.Out, env)
	} else {
		_ = output.WriteAnalyze(a.io.Out, result)
	}

	if failure {
		if global.JSON {
			return result, result.Findings, ExitPolicyViolation, markReportedError(failErr)
		}
		return result, result.Findings, ExitPolicyViolation, failErr
	}

	return result, result.Findings, ExitOK, nil
}

func (a *App) runBuild(ctx context.Context, global GlobalOptions, loaded config.Loaded, args []string) (*backend.BuildResult, int, error) {
	allowed, err := a.capabilities.Has(ctx, capabilities.FeatureBuild)
	if err != nil {
		return nil, ExitAuthDenied, err
	}
	if !allowed {
		return nil, ExitAuthDenied, fmt.Errorf("capability denied: build")
	}

	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	contextDir := fs.String("context", ".", "Build context path")
	dockerfile := fs.String("file", loaded.Config.Defaults.Build.Dockerfile, "Dockerfile path")
	target := fs.String("target", "", "Build stage target")
	platforms := stringSliceFlag{}
	buildArgs := kvSliceFlag{}
	secrets := kvSliceFlag{}
	outputMode := fs.String("output", loaded.Config.Defaults.Build.OutputMode, "Output mode: image|oci|local")
	imageRef := fs.String("image-ref", loaded.Config.Defaults.Build.ImageRef, "Image reference for image output")
	ociDest := fs.String("oci-dest", "", "Destination path for OCI tar")
	localDest := fs.String("local-dest", "", "Destination directory for local output")
	backendName := fs.String("backend", loaded.Config.Backend, "Backend selector")
	endpoint := fs.String("endpoint", loaded.Config.Endpoint, "BuildKit endpoint")

	fs.Var(&platforms, "platform", "Target platform (repeatable)")
	fs.Var(&buildArgs, "build-arg", "Build arg key=value (repeatable)")
	fs.Var(&secrets, "secret", "Build secret id=...,src=... (repeatable)")

	if err := fs.Parse(args); err != nil {
		return nil, ExitUsage, err
	}

	selectedBackend, err := a.resolveBackend(*backendName)
	if err != nil {
		return nil, ExitBackend, err
	}

	progress := func(event backend.BuildProgressEvent) {
		if global.JSON {
			return
		}
		fmt.Fprintf(a.io.Err, "[%s] %s\n", event.Phase, strings.TrimSpace(event.Message))
	}

	result, err := selectedBackend.Build(ctx, backend.BuildRequest{
		ContextDir:        *contextDir,
		Dockerfile:        *dockerfile,
		Target:            *target,
		Platforms:         platforms,
		BuildArgs:         buildArgs.toBuildArgs(),
		Secrets:           parseSecrets(secrets),
		OutputMode:        strings.ToLower(strings.TrimSpace(*outputMode)),
		ImageRef:          *imageRef,
		OCIDest:           *ociDest,
		LocalDest:         *localDest,
		Backend:           *backendName,
		Endpoint:          *endpoint,
		ProjectConfigPath: loaded.Paths.ProjectPath,
		GlobalConfigPath:  loaded.Paths.GlobalPath,
	}, progress)
	if err != nil {
		return nil, ExitBuildFailed, err
	}

	if global.JSON {
		_ = output.WriteJSON(a.io.Out, output.Envelope{
			APIVersion: output.APIVersion,
			Command:    "build",
			Timestamp:  time.Now().UTC(),
			DurationMS: 0,
			Result:     result,
		})
	} else {
		_ = output.WriteBuild(a.io.Out, result)
	}
	return &result, ExitOK, nil
}

func (a *App) runBackend(global GlobalOptions, args []string) (int, error) {
	if len(args) == 0 {
		return ExitUsage, fmt.Errorf("backend subcommand is required")
	}
	if args[0] != "list" {
		return ExitUsage, fmt.Errorf("unsupported backend subcommand %q", args[0])
	}
	names := a.registry.List()
	if global.JSON {
		return ExitOK, output.WriteJSON(a.io.Out, output.Envelope{
			APIVersion: output.APIVersion,
			Command:    "backend list",
			Timestamp:  time.Now().UTC(),
			DurationMS: 0,
			Result: map[string]any{
				"backends": names,
			},
		})
	}
	for _, name := range names {
		fmt.Fprintln(a.io.Out, name)
	}
	return ExitOK, nil
}

func (a *App) runDoctor(ctx context.Context, global GlobalOptions, loaded config.Loaded, store *state.Store) (int, error) {
	checks := map[string]string{
		"config.global":  status(loaded.Paths.GlobalExists, loaded.Paths.GlobalPath),
		"config.project": status(loaded.Paths.ProjectExists, loaded.Paths.ProjectPath),
	}
	if store != nil {
		checks["state.sqlite"] = "ok: " + store.Path()
	} else {
		checks["state.sqlite"] = "error: unavailable"
	}

	selectedBackend, err := a.resolveBackend(loaded.Config.Backend)
	if err != nil {
		checks["backend.detect"] = "error: " + err.Error()
	} else {
		detect, detectErr := selectedBackend.Detect(ctx, backend.DetectRequest{
			Backend:           loaded.Config.Backend,
			Endpoint:          loaded.Config.Endpoint,
			ProjectConfigPath: loaded.Paths.ProjectPath,
			GlobalConfigPath:  loaded.Paths.GlobalPath,
		})
		if detectErr != nil {
			checks["backend.detect"] = "error: " + detectErr.Error()
		} else {
			checks["backend.detect"] = fmt.Sprintf("ok: mode=%s endpoint=%s", detect.Mode, detect.Endpoint)
		}
	}

	if global.JSON {
		if err := output.WriteJSON(a.io.Out, output.Envelope{
			APIVersion: output.APIVersion,
			Command:    "doctor",
			Timestamp:  time.Now().UTC(),
			DurationMS: 0,
			Result: map[string]any{
				"checks": checks,
			},
		}); err != nil {
			return ExitInternal, err
		}
	} else {
		if err := output.WriteDoctor(a.io.Out, checks); err != nil {
			return ExitInternal, err
		}
	}

	for _, value := range checks {
		if strings.HasPrefix(value, "error:") {
			err := fmt.Errorf("doctor detected failing checks")
			if global.JSON {
				return ExitBackend, markReportedError(err)
			}
			return ExitBackend, err
		}
	}
	return ExitOK, nil
}

func (a *App) runAuth(global GlobalOptions, args []string) (int, error) {
	if len(args) == 0 {
		return ExitUsage, fmt.Errorf("auth subcommand is required")
	}
	manager, err := a.authManager()
	if err != nil {
		return ExitConfigState, err
	}

	subcommand := args[0]
	subArgs := args[1:]
	fs := flag.NewFlagSet("auth "+subcommand, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	user := fs.String("user", "", "Auth user")
	token := fs.String("token", "", "Auth token")

	switch subcommand {
	case "login":
		if err := fs.Parse(subArgs); err != nil {
			return ExitUsage, err
		}
		if strings.TrimSpace(*user) == "" || strings.TrimSpace(*token) == "" {
			return ExitUsage, fmt.Errorf("--user and --token are required for auth login")
		}
		if err := manager.Save(auth.Credentials{User: *user, Token: *token}); err != nil {
			return ExitConfigState, err
		}
		return a.writeSimple(global.JSON, "auth login", map[string]any{"status": "logged-in", "user": *user})
	case "logout":
		if err := manager.Delete(); err != nil {
			return ExitConfigState, err
		}
		return a.writeSimple(global.JSON, "auth logout", map[string]any{"status": "logged-out"})
	case "whoami":
		creds, err := manager.Load()
		if err != nil {
			return ExitAuthDenied, fmt.Errorf("not logged in")
		}
		return a.writeSimple(global.JSON, "auth whoami", map[string]any{"user": creds.User, "source": creds.Source, "storedAt": creds.StoredAt})
	default:
		return ExitUsage, fmt.Errorf("unsupported auth subcommand %q", subcommand)
	}
}

func (a *App) runConfig(global GlobalOptions, loaded config.Loaded, args []string) (int, error) {
	if len(args) == 0 || args[0] != "show" {
		return ExitUsage, fmt.Errorf("supported config command: show")
	}
	if global.JSON {
		return ExitOK, output.WriteJSON(a.io.Out, output.Envelope{
			APIVersion: output.APIVersion,
			Command:    "config show",
			Timestamp:  time.Now().UTC(),
			DurationMS: 0,
			Result:     loaded,
		})
	}

	fmt.Fprintf(a.io.Out, "Global config: %s\n", loaded.Paths.GlobalPath)
	fmt.Fprintf(a.io.Out, "Project config: %s\n", loaded.Paths.ProjectPath)
	fmt.Fprintf(a.io.Out, "Backend: %s\n", loaded.Config.Backend)
	fmt.Fprintf(a.io.Out, "Endpoint: %s\n", loaded.Config.Endpoint)
	fmt.Fprintf(a.io.Out, "Telemetry enabled: %t\n", loaded.Config.Telemetry.Enabled)
	return ExitOK, nil
}

func (a *App) runVersion(global GlobalOptions) (int, error) {
	payload := map[string]any{
		"version":   version.Version,
		"commit":    version.Commit,
		"buildDate": version.BuildDate,
	}
	return a.writeSimple(global.JSON, "version", payload)
}

func (a *App) writeSimple(asJSON bool, command string, result any) (int, error) {
	if asJSON {
		if err := output.WriteJSON(a.io.Out, output.Envelope{
			APIVersion: output.APIVersion,
			Command:    command,
			Timestamp:  time.Now().UTC(),
			DurationMS: 0,
			Result:     result,
		}); err != nil {
			return ExitInternal, err
		}
	} else {
		keys := make([]string, 0)
		if m, ok := result.(map[string]any); ok {
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, key := range keys {
				fmt.Fprintf(a.io.Out, "%s: %v\n", key, m[key])
			}
		}
	}
	return ExitOK, nil
}

func (a *App) resolveBackend(requested string) (backend.Backend, error) {
	name := strings.TrimSpace(strings.ToLower(requested))
	if name == "" || name == "auto" {
		name = buildkit.BackendName
	}
	selected, ok := a.registry.Get(name)
	if !ok {
		return nil, fmt.Errorf("backend %q is not registered", name)
	}
	return selected, nil
}

func (a *App) openStateStore() (*state.Store, error) {
	path, err := config.DefaultStateDBPath()
	if err != nil {
		return nil, err
	}
	if err := config.EnsureParent(path); err != nil {
		return nil, err
	}
	return state.Open(path)
}

func (a *App) authManager() (*auth.Manager, error) {
	configPath, err := config.DefaultGlobalConfigPath()
	if err != nil {
		return nil, err
	}
	authPath := filepath.Join(filepath.Dir(configPath), "auth.json")
	return auth.NewManager(authPath)
}

func parseGlobalFlags(args []string, stderr io.Writer) (GlobalOptions, []string, error) {
	opts := GlobalOptions{}
	remaining := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			remaining = append(remaining, args[i:]...)
			break
		}

		name, value, hasInlineValue := splitFlag(arg)
		switch name {
		case "--json":
			parsed, err := parseOptionalBool(value, hasInlineValue, true)
			if err != nil {
				fmt.Fprintln(stderr, err)
				return opts, nil, err
			}
			opts.JSON = parsed
		case "--no-color":
			parsed, err := parseOptionalBool(value, hasInlineValue, true)
			if err != nil {
				fmt.Fprintln(stderr, err)
				return opts, nil, err
			}
			opts.NoColor = parsed
		case "--verbose":
			parsed, err := parseOptionalBool(value, hasInlineValue, true)
			if err != nil {
				fmt.Fprintln(stderr, err)
				return opts, nil, err
			}
			opts.Verbose = parsed
		case "--non-interactive":
			parsed, err := parseOptionalBool(value, hasInlineValue, true)
			if err != nil {
				fmt.Fprintln(stderr, err)
				return opts, nil, err
			}
			opts.NonInteractive = parsed
		case "--config":
			if hasInlineValue {
				opts.ConfigPath = value
				continue
			}
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("--config requires a value")
			}
			i++
			opts.ConfigPath = args[i]
		case "--project-config":
			if hasInlineValue {
				opts.ProjectConfig = value
				continue
			}
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("--project-config requires a value")
			}
			i++
			opts.ProjectConfig = args[i]
		case "--profile":
			if hasInlineValue {
				opts.Profile = value
				continue
			}
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("--profile requires a value")
			}
			i++
			opts.Profile = args[i]
		default:
			remaining = append(remaining, arg)
		}
	}
	return opts, remaining, nil
}

func splitFlag(arg string) (name, value string, hasInlineValue bool) {
	if !strings.HasPrefix(arg, "--") {
		return arg, "", false
	}
	parts := strings.SplitN(arg, "=", 2)
	if len(parts) == 2 {
		return parts[0], parts[1], true
	}
	return arg, "", false
}

func parseOptionalBool(value string, hasValue bool, defaultValue bool) (bool, error) {
	if !hasValue {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("invalid boolean value %q", value)
	}
	return parsed, nil
}

func shouldFailFindings(findings []backend.Finding, failOn string) bool {
	if len(findings) == 0 {
		return false
	}
	switch failOn {
	case "", "any":
		return true
	case "security":
		for _, finding := range findings {
			if finding.Dimension == backend.DimensionSecurity {
				return true
			}
		}
	case "policy":
		for _, finding := range findings {
			if finding.Dimension == backend.DimensionPolicy {
				return true
			}
		}
	}
	return false
}

func normalizeSeverity(value string) string {
	s := strings.ToLower(strings.TrimSpace(value))
	switch s {
	case backend.SeverityLow, backend.SeverityMedium, backend.SeverityHigh, backend.SeverityCritical:
		return s
	default:
		return backend.SeverityLow
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func status(ok bool, detail string) string {
	if ok {
		return "ok: " + detail
	}
	return "missing: " + detail
}

type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSliceFlag) Set(value string) error {
	for _, item := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		*s = append(*s, trimmed)
	}
	return nil
}

type kvSliceFlag []string

func (k *kvSliceFlag) String() string {
	return strings.Join(*k, ",")
}

func (k *kvSliceFlag) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	*k = append(*k, strings.TrimSpace(value))
	return nil
}

func (k kvSliceFlag) toBuildArgs() []backend.BuildArg {
	args := make([]backend.BuildArg, 0, len(k))
	for _, item := range k {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			continue
		}
		args = append(args, backend.BuildArg{Key: parts[0], Value: parts[1]})
	}
	return args
}

func parseSecrets(values kvSliceFlag) []backend.SecretSpec {
	secrets := make([]backend.SecretSpec, 0, len(values))
	for _, item := range values {
		parts := strings.Split(item, ",")
		secret := backend.SecretSpec{}
		for _, part := range parts {
			pair := strings.SplitN(strings.TrimSpace(part), "=", 2)
			if len(pair) != 2 {
				continue
			}
			switch pair[0] {
			case "id":
				secret.ID = pair[1]
			case "src":
				secret.Src = pair[1]
			}
		}
		if secret.ID != "" {
			secrets = append(secrets, secret)
		}
	}
	return secrets
}

func (a *App) writeError(asJSON bool, command string, durationMs int64, err error) {
	if err == nil {
		return
	}
	if asJSON {
		_ = output.WriteJSON(a.io.Err, output.Envelope{
			APIVersion: output.APIVersion,
			Command:    command,
			Timestamp:  time.Now().UTC(),
			DurationMS: durationMs,
			Errors: []output.ErrorItem{{
				Code:    "error",
				Message: err.Error(),
			}},
		})
		return
	}
	fmt.Fprintf(a.io.Err, "Error: %v\n", err)
}

func (a *App) printHelp(w io.Writer) {
	fmt.Fprintln(w, "buildgraph - Build intelligence CLI")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  buildgraph [global flags] <command> [command flags]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  analyze       Analyze Dockerfile and build context")
	fmt.Fprintln(w, "  build         Execute BuildKit build")
	fmt.Fprintln(w, "  backend list  List available backends")
	fmt.Fprintln(w, "  doctor        Run environment diagnostics")
	fmt.Fprintln(w, "  auth          Manage SaaS authentication state")
	fmt.Fprintln(w, "  config show   Show effective config")
	fmt.Fprintln(w, "  version       Print version information")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Global Flags:")
	fmt.Fprintln(w, "  --json             Render JSON output")
	fmt.Fprintln(w, "  --no-color         Disable color")
	fmt.Fprintln(w, "  --verbose          Verbose mode")
	fmt.Fprintln(w, "  --config PATH      Global config path")
	fmt.Fprintln(w, "  --project-config PATH  Project config path")
	fmt.Fprintln(w, "  --profile NAME     Config profile")
	fmt.Fprintln(w, "  --non-interactive  Disable prompts")
}

type reportedError struct {
	err error
}

func (e reportedError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e reportedError) Unwrap() error {
	return e.err
}

func markReportedError(err error) error {
	if err == nil {
		return nil
	}
	return reportedError{err: err}
}

func isReportedError(err error) bool {
	var wrapped reportedError
	return errors.As(err, &wrapped)
}
