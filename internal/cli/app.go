package cli

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Makepad-fr/buildgraph/internal/analyze"
	"github.com/Makepad-fr/buildgraph/internal/backend"
	"github.com/Makepad-fr/buildgraph/internal/backend/buildkit"
	"github.com/Makepad-fr/buildgraph/internal/config"
	"github.com/Makepad-fr/buildgraph/internal/output"
	"github.com/Makepad-fr/buildgraph/internal/platform/auth"
	"github.com/Makepad-fr/buildgraph/internal/platform/capabilities"
	"github.com/Makepad-fr/buildgraph/internal/platform/events"
	"github.com/Makepad-fr/buildgraph/internal/report"
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
		a.writeError(global.JSON, "config", err)
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
	var recordReport *report.BuildReport
	commandLabel := command

	var emit events.Sink = events.NoopSink{}
	if stateStore != nil {
		emit = events.MultiSink{Sinks: []events.Sink{events.NoopSink{}, events.LocalSink{Recorder: stateStore}}}
	}

	switch command {
	case "help", "--help", "-h":
		a.printHelp(a.io.Out)
	case "analyze":
		cmd, findings, buildResult, runReport, code, err := a.runAnalyze(ctx, global, loadedCfg, stateStore, cmdArgs)
		commandLabel = cmd
		exitCode = code
		runErr = err
		recordFindings = findings
		recordBuild = buildResult
		recordReport = runReport
		if err == nil {
			payload := any(runReport)
			if payload == nil {
				payload = map[string]any{"findingCount": len(findings)}
			}
			_ = emit.Emit(ctx, events.Event{Name: cmd + ".completed", Payload: payload, CreatedAt: time.Now().UTC()})
		}
	case "build":
		result, code, err := a.runBuild(ctx, global, loadedCfg, cmdArgs)
		commandLabel = "build"
		exitCode = code
		runErr = err
		recordBuild = result
		if err == nil && result != nil {
			_ = emit.Emit(ctx, events.Event{Name: "build.completed", Payload: result, CreatedAt: time.Now().UTC()})
		}
	case "backend":
		commandLabel = "backend"
		exitCode, runErr = a.runBackend(global, cmdArgs)
	case "doctor":
		commandLabel = "doctor"
		exitCode, runErr = a.runDoctor(ctx, global, loadedCfg, stateStore)
	case "report":
		commandLabel = "report"
		exitCode, runErr = a.runReport(ctx, global, loadedCfg, stateStore, cmdArgs)
	case "ci":
		commandLabel = "ci"
		exitCode, runErr = a.runCI(ctx, global, loadedCfg, stateStore, cmdArgs)
	case "auth":
		commandLabel = "auth"
		exitCode, runErr = a.runAuth(global, cmdArgs)
	case "config":
		commandLabel = "config"
		exitCode, runErr = a.runConfig(global, loadedCfg, cmdArgs)
	case "version":
		commandLabel = "version"
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
			if recordReport != nil {
				reportCopy := *recordReport
				reportCopy.RunID = runID
				_ = stateStore.RecordReport(ctx, runID, "BuildReport", reportCopy)
			}
		}
	}

	if runErr != nil {
		if !isReportedError(runErr) {
			a.writeError(global.JSON, commandLabel, runErr)
		}
	}

	return exitCode
}

func (a *App) runAnalyze(ctx context.Context, global GlobalOptions, loaded config.Loaded, store *state.Store, args []string) (string, []backend.Finding, *backend.BuildResult, *report.BuildReport, int, error) {
	if len(args) > 0 && args[0] == "run" {
		rep, findings, buildResult, code, err := a.runAnalyzeRun(ctx, global, loaded, args[1:])
		return "analyze run", findings, buildResult, rep, code, err
	}

	allowed, err := a.capabilities.Has(ctx, capabilities.FeatureAnalyze)
	if err != nil {
		return "analyze", nil, nil, nil, ExitAuthDenied, err
	}
	if !allowed {
		return "analyze", nil, nil, nil, ExitAuthDenied, fmt.Errorf("capability denied: analyze")
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
		return "analyze", nil, nil, nil, ExitUsage, err
	}

	selectedBackend, err := a.resolveBackend(*backendName)
	if err != nil {
		return "analyze", nil, nil, nil, ExitBackend, err
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
		return "analyze", nil, nil, nil, ExitBackend, err
	}

	failure := shouldFailFindings(result.Findings, strings.ToLower(strings.TrimSpace(*failOn)))
	failErr := fmt.Errorf("analysis found violations matching fail-on=%s", *failOn)

	summary := map[string]any{
		"findingCount": len(result.Findings),
		"backend":      result.Backend,
		"endpoint":     result.Endpoint,
	}
	spec := map[string]any{
		"context":           *contextDir,
		"file":              *dockerfile,
		"severityThreshold": *severityThreshold,
		"failOn":            *failOn,
		"backend":           *backendName,
		"endpoint":          *endpoint,
	}
	if global.JSON {
		if failure {
			resource := output.Resource{
				APIVersion: output.APIVersion,
				Kind:       "AnalyzeReport",
				Metadata:   output.ResourceMetadata{Command: "analyze", GeneratedAt: time.Now().UTC()},
				Spec:       spec,
				Status: output.ResourceStatus{
					Phase:   "failed",
					Summary: summary,
					Result:  result,
					Errors:  []output.ErrorItem{{Code: "violation", Message: failErr.Error()}},
				},
			}
			_ = output.WriteJSON(a.io.Out, resource)
		} else {
			_ = output.WriteJSON(a.io.Out, output.SuccessResource("AnalyzeReport", "analyze", spec, summary, result, 0))
		}
	} else {
		_ = output.WriteAnalyze(a.io.Out, result)
	}

	if failure {
		if global.JSON {
			return "analyze", result.Findings, nil, nil, ExitPolicyViolation, markReportedError(failErr)
		}
		return "analyze", result.Findings, nil, nil, ExitPolicyViolation, failErr
	}
	return "analyze", result.Findings, nil, nil, ExitOK, nil
}

func (a *App) runAnalyzeRun(ctx context.Context, global GlobalOptions, loaded config.Loaded, args []string) (*report.BuildReport, []backend.Finding, *backend.BuildResult, int, error) {
	allowedAnalyze, err := a.capabilities.Has(ctx, capabilities.FeatureAnalyze)
	if err != nil {
		return nil, nil, nil, ExitAuthDenied, err
	}
	if !allowedAnalyze {
		return nil, nil, nil, ExitAuthDenied, fmt.Errorf("capability denied: analyze")
	}
	allowedBuild, err := a.capabilities.Has(ctx, capabilities.FeatureBuild)
	if err != nil {
		return nil, nil, nil, ExitAuthDenied, err
	}
	if !allowedBuild {
		return nil, nil, nil, ExitAuthDenied, fmt.Errorf("capability denied: build")
	}

	fs := flag.NewFlagSet("analyze run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	contextDir := fs.String("context", ".", "Build context path")
	dockerfile := fs.String("file", loaded.Config.Defaults.Analyze.Dockerfile, "Dockerfile path")
	severityThreshold := fs.String("severity-threshold", loaded.Config.Defaults.Analyze.SeverityThreshold, "Minimum severity: low|medium|high|critical")
	failOn := fs.String("fail-on", loaded.Config.Defaults.Analyze.FailOn, "Failure mode: policy|security|any")
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
		return nil, nil, nil, ExitUsage, err
	}

	selectedBackend, err := a.resolveBackend(*backendName)
	if err != nil {
		return nil, nil, nil, ExitBackend, err
	}

	progress := func(event backend.BuildProgressEvent) {
		if global.JSON {
			return
		}
		fmt.Fprintf(a.io.Err, "[%s] %s\n", event.Phase, strings.TrimSpace(event.Message))
	}

	buildResult, err := selectedBackend.Build(ctx, backend.BuildRequest{
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
		return nil, nil, nil, ExitBuildFailed, err
	}

	analyzeResult, err := selectedBackend.Analyze(ctx, backend.AnalyzeRequest{
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
		return nil, nil, nil, ExitBackend, err
	}

	stageGraph, stageErr := analyze.ParseStageGraph(*contextDir, *dockerfile)
	if stageErr != nil {
		stageGraph = analyze.StageGraph{}
		buildResult.Warnings = append(buildResult.Warnings, "unable to parse stage graph: "+stageErr.Error())
	}

	runReport := report.NewBuildReport(report.BuildReportInput{
		RunID:       0,
		Command:     "analyze run",
		ContextDir:  *contextDir,
		Dockerfile:  *dockerfile,
		Build:       buildResult,
		Findings:    analyzeResult.Findings,
		StageGraph:  stageGraph,
		GeneratedAt: time.Now().UTC(),
	})

	spec := map[string]any{
		"context":           *contextDir,
		"file":              *dockerfile,
		"backend":           *backendName,
		"endpoint":          *endpoint,
		"severityThreshold": *severityThreshold,
		"failOn":            *failOn,
		"target":            *target,
		"platform":          []string(platforms),
		"output":            *outputMode,
	}

	failure := shouldFailFindings(analyzeResult.Findings, strings.ToLower(strings.TrimSpace(*failOn)))
	failErr := fmt.Errorf("analysis found violations matching fail-on=%s", *failOn)

	if global.JSON {
		if failure {
			resource := output.Resource{
				APIVersion: output.APIVersion,
				Kind:       "BuildReport",
				Metadata:   output.ResourceMetadata{Command: "analyze run", GeneratedAt: time.Now().UTC()},
				Spec:       spec,
				Status: output.ResourceStatus{
					Phase:   "failed",
					Summary: runReport.Summary,
					Result:  runReport,
					Errors:  []output.ErrorItem{{Code: "violation", Message: failErr.Error()}},
				},
			}
			_ = output.WriteJSON(a.io.Out, resource)
		} else {
			_ = output.WriteJSON(a.io.Out, output.SuccessResource("BuildReport", "analyze run", spec, runReport.Summary, runReport, 0))
		}
	} else {
		_ = output.WriteBuildReport(a.io.Out, runReport)
	}

	if failure {
		if global.JSON {
			return &runReport, analyzeResult.Findings, &buildResult, ExitPolicyViolation, markReportedError(failErr)
		}
		return &runReport, analyzeResult.Findings, &buildResult, ExitPolicyViolation, failErr
	}

	return &runReport, analyzeResult.Findings, &buildResult, ExitOK, nil
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
		summary := map[string]any{
			"outputs":     len(result.Outputs),
			"cacheHits":   result.CacheStats.Hits,
			"cacheMisses": result.CacheStats.Misses,
		}
		spec := map[string]any{
			"context":  *contextDir,
			"file":     *dockerfile,
			"backend":  *backendName,
			"endpoint": *endpoint,
			"output":   *outputMode,
		}
		_ = output.WriteJSON(a.io.Out, output.SuccessResource("BuildExecutionReport", "build", spec, summary, result, 0))
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
		summary := map[string]any{"count": len(names)}
		result := map[string]any{"backends": names}
		return ExitOK, output.WriteJSON(a.io.Out, output.SuccessResource("BackendList", "backend list", nil, summary, result, 0))
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
	doctorReport := output.DoctorReport{
		Checks: checks,
		CommonFixes: []string{
			"Set --endpoint or BUILDKIT_HOST to a reachable BuildKit daemon.",
			"Run buildkitd locally or start Docker Desktop with BuildKit enabled.",
			"Set ci.baselineSource with matching baselineFile/baselineUrl in config for CI checks.",
		},
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
			doctorReport.Found = detect
			doctorReport.Attempts = detect.Attempts
			doctorReport.ConfigSnippet = fmt.Sprintf("backend: buildkit\nendpoint: %q\n", detect.Endpoint)
		}
	}

	if version, err := report.GraphvizVersion(); err != nil {
		checks["graphviz.dot"] = "error: dot not found"
	} else {
		checks["graphviz.dot"] = "ok: " + version
	}

	source := strings.TrimSpace(loaded.Config.CI.BaselineSource)
	if source == "" {
		checks["ci.baseline"] = "missing: baselineSource not configured"
	} else {
		switch source {
		case "git", "ci-artifact":
			if strings.TrimSpace(loaded.Config.CI.BaselineFile) == "" {
				checks["ci.baseline"] = "error: baselineFile required for source=" + source
			} else {
				checks["ci.baseline"] = "ok: source=" + source
			}
		case "object-storage":
			if strings.TrimSpace(loaded.Config.CI.BaselineURL) == "" {
				checks["ci.baseline"] = "error: baselineUrl required for source=object-storage"
			} else {
				checks["ci.baseline"] = "ok: source=object-storage"
			}
		default:
			checks["ci.baseline"] = "error: unsupported source=" + source
		}
	}

	summary := map[string]any{"checkCount": len(checks)}
	if global.JSON {
		if err := output.WriteJSON(a.io.Out, output.SuccessResource("DoctorReport", "doctor", nil, summary, doctorReport, 0)); err != nil {
			return ExitInternal, err
		}
	} else {
		if err := output.WriteDoctor(a.io.Out, doctorReport); err != nil {
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

func (a *App) runReport(ctx context.Context, global GlobalOptions, loaded config.Loaded, store *state.Store, args []string) (int, error) {
	if len(args) == 0 {
		return ExitUsage, fmt.Errorf("report subcommand is required")
	}
	switch args[0] {
	case "show":
		return a.runReportShow(ctx, global, store, args[1:])
	case "metrics":
		return a.runReportMetrics(ctx, global, store, args[1:])
	case "compare":
		return a.runReportCompare(ctx, global, loaded, store, args[1:])
	case "trend":
		return a.runReportTrend(ctx, global, store, args[1:])
	case "export":
		return a.runReportExport(ctx, global, store, args[1:])
	default:
		return ExitUsage, fmt.Errorf("unsupported report subcommand %q", args[0])
	}
}

func (a *App) runReportShow(ctx context.Context, global GlobalOptions, store *state.Store, args []string) (int, error) {
	fs := flag.NewFlagSet("report show", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	runID := fs.Int64("run-id", 0, "Run ID")
	file := fs.String("file", "", "Path to BuildReport JSON")
	if err := fs.Parse(args); err != nil {
		return ExitUsage, err
	}

	runReport, source, err := a.resolveReportSource(ctx, store, *runID, *file)
	if err != nil {
		return ExitConfigState, err
	}
	if global.JSON {
		spec := map[string]any{"source": source}
		if err := output.WriteJSON(a.io.Out, output.SuccessResource("BuildReport", "report show", spec, runReport.Summary, runReport, runReport.RunID)); err != nil {
			return ExitInternal, err
		}
	} else {
		if err := output.WriteBuildReport(a.io.Out, runReport); err != nil {
			return ExitInternal, err
		}
	}
	return ExitOK, nil
}

func (a *App) runReportMetrics(ctx context.Context, global GlobalOptions, store *state.Store, args []string) (int, error) {
	fs := flag.NewFlagSet("report metrics", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	runID := fs.Int64("run-id", 0, "Run ID")
	file := fs.String("file", "", "Path to BuildReport JSON")
	if err := fs.Parse(args); err != nil {
		return ExitUsage, err
	}

	runReport, source, err := a.resolveReportSource(ctx, store, *runID, *file)
	if err != nil {
		return ExitConfigState, err
	}
	if global.JSON {
		spec := map[string]any{"source": source}
		if err := output.WriteJSON(a.io.Out, output.SuccessResource("MetricsReport", "report metrics", spec, runReport.Summary, runReport.Metrics, runReport.RunID)); err != nil {
			return ExitInternal, err
		}
	} else {
		if _, err := fmt.Fprintf(a.io.Out, "Critical path: %dms\nCache hit ratio: %.2f%%\n", runReport.Metrics.CriticalPathMS, runReport.Metrics.CacheHitRatio*100); err != nil {
			return ExitInternal, err
		}
		if len(runReport.Metrics.TopSlowVertices) > 0 {
			if _, err := fmt.Fprintln(a.io.Out, "Top slow vertices:"); err != nil {
				return ExitInternal, err
			}
			for _, vertex := range runReport.Metrics.TopSlowVertices {
				if _, err := fmt.Fprintf(a.io.Out, "- %s (%dms)\n", strings.TrimSpace(vertex.Name), vertex.DurationMS); err != nil {
					return ExitInternal, err
				}
			}
		}
	}
	return ExitOK, nil
}

func (a *App) runReportCompare(ctx context.Context, global GlobalOptions, loaded config.Loaded, store *state.Store, args []string) (int, error) {
	fs := flag.NewFlagSet("report compare", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	baseSource := fs.String("base", "", "Base report source (run:<id> or file path)")
	headSource := fs.String("head", "", "Head report source (run:<id> or file path)")
	thresholds := thresholdFlag{}
	fs.Var(&thresholds, "threshold", "Threshold override key=value (repeatable)")
	if err := fs.Parse(args); err != nil {
		return ExitUsage, err
	}
	if strings.TrimSpace(*baseSource) == "" || strings.TrimSpace(*headSource) == "" {
		return ExitUsage, fmt.Errorf("--base and --head are required")
	}

	baseReport, baseRef, err := a.resolveReportNamedSource(ctx, store, *baseSource)
	if err != nil {
		return ExitConfigState, err
	}
	headReport, headRef, err := a.resolveReportNamedSource(ctx, store, *headSource)
	if err != nil {
		return ExitConfigState, err
	}

	effectiveThresholds := mergeThresholdMaps(loaded.Config.CI.Thresholds, thresholds)
	cmp := report.Compare(baseReport, headReport, effectiveThresholds, baseRef, headRef)

	if global.JSON {
		spec := map[string]any{"base": baseRef, "head": headRef, "thresholds": effectiveThresholds}
		phase := "completed"
		if !cmp.Passed {
			phase = "failed"
		}
		resource := output.Resource{
			APIVersion: output.APIVersion,
			Kind:       "CompareReport",
			Metadata:   output.ResourceMetadata{Command: "report compare", GeneratedAt: time.Now().UTC()},
			Spec:       spec,
			Status: output.ResourceStatus{
				Phase:   phase,
				Summary: map[string]any{"passed": cmp.Passed, "regressionCount": len(cmp.Regressions)},
				Result:  cmp,
			},
		}
		if !cmp.Passed {
			resource.Status.Errors = []output.ErrorItem{{Code: "regression", Message: strings.Join(cmp.Regressions, "; ")}}
		}
		if err := output.WriteJSON(a.io.Out, resource); err != nil {
			return ExitInternal, err
		}
	} else {
		if err := output.WriteCompareReport(a.io.Out, cmp); err != nil {
			return ExitInternal, err
		}
	}

	if !cmp.Passed {
		err := fmt.Errorf("regressions detected")
		if global.JSON {
			return ExitPolicyViolation, markReportedError(err)
		}
		return ExitPolicyViolation, err
	}
	return ExitOK, nil
}

func (a *App) runReportTrend(ctx context.Context, global GlobalOptions, store *state.Store, args []string) (int, error) {
	if store == nil {
		return ExitConfigState, fmt.Errorf("state store unavailable")
	}
	fs := flag.NewFlagSet("report trend", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	last := fs.Int("last", 10, "Number of recent runs")
	_ = fs.String("branch", "", "Branch hint (reserved)")
	if err := fs.Parse(args); err != nil {
		return ExitUsage, err
	}

	recs, err := store.ListRecentReports(ctx, *last)
	if err != nil {
		return ExitConfigState, err
	}
	reports := make([]report.BuildReport, 0, len(recs))
	for i := len(recs) - 1; i >= 0; i-- {
		runReport, err := report.ParseBuildReportJSON([]byte(recs[i].ReportJSON))
		if err != nil {
			continue
		}
		runReport.RunID = recs[i].RunID
		reports = append(reports, runReport)
	}
	trend := report.BuildTrend(reports)

	if global.JSON {
		spec := map[string]any{"last": *last}
		if err := output.WriteJSON(a.io.Out, output.SuccessResource("TrendReport", "report trend", spec, map[string]any{"window": trend.Window}, trend, 0)); err != nil {
			return ExitInternal, err
		}
	} else {
		if err := output.WriteTrendReport(a.io.Out, trend); err != nil {
			return ExitInternal, err
		}
	}
	return ExitOK, nil
}

func (a *App) runReportExport(ctx context.Context, global GlobalOptions, store *state.Store, args []string) (int, error) {
	fs := flag.NewFlagSet("report export", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	runID := fs.Int64("run-id", 0, "Run ID")
	file := fs.String("file", "", "Path to BuildReport JSON")
	format := fs.String("format", "dot", "Output format: dot|svg")
	out := fs.String("out", "", "Output path")
	if err := fs.Parse(args); err != nil {
		return ExitUsage, err
	}
	if strings.TrimSpace(*out) == "" {
		return ExitUsage, fmt.Errorf("--out is required")
	}

	runReport, source, err := a.resolveReportSource(ctx, store, *runID, *file)
	if err != nil {
		return ExitConfigState, err
	}

	dot := report.RenderDOT(runReport)
	formatValue := strings.ToLower(strings.TrimSpace(*format))
	switch formatValue {
	case "dot":
		if err := os.WriteFile(*out, []byte(dot), 0o644); err != nil {
			return ExitConfigState, fmt.Errorf("write dot export: %w", err)
		}
	case "svg":
		if err := report.RenderSVG(dot, *out); err != nil {
			return ExitBackend, err
		}
	default:
		return ExitUsage, fmt.Errorf("unsupported export format %q", *format)
	}

	result := map[string]any{"out": *out, "format": formatValue, "source": source}
	if global.JSON {
		if err := output.WriteJSON(a.io.Out, output.SuccessResource("GraphExportReport", "report export", nil, nil, result, runReport.RunID)); err != nil {
			return ExitInternal, err
		}
	} else {
		_, _ = fmt.Fprintf(a.io.Out, "Exported %s graph to %s\n", formatValue, *out)
	}
	return ExitOK, nil
}

func (a *App) runCI(ctx context.Context, global GlobalOptions, loaded config.Loaded, store *state.Store, args []string) (int, error) {
	if len(args) == 0 {
		return ExitUsage, fmt.Errorf("ci subcommand is required")
	}
	switch args[0] {
	case "check":
		return a.runCICheck(ctx, global, loaded, store, args[1:])
	case "github-action":
		return a.runCIGitHubAction(global, args[1:])
	case "gitlab-ci":
		return a.runCIGitLab(global, args[1:])
	default:
		return ExitUsage, fmt.Errorf("unsupported ci subcommand %q", args[0])
	}
}

func (a *App) runCICheck(ctx context.Context, global GlobalOptions, loaded config.Loaded, store *state.Store, args []string) (int, error) {
	fs := flag.NewFlagSet("ci check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	baselineSource := fs.String("baseline-source", loaded.Config.CI.BaselineSource, "Baseline source: git|ci-artifact|object-storage")
	baselineFile := fs.String("baseline-file", loaded.Config.CI.BaselineFile, "Baseline report file")
	baselineURL := fs.String("baseline-url", loaded.Config.CI.BaselineURL, "Baseline report URL")
	headRunID := fs.Int64("head-run-id", 0, "Head run ID")
	headFile := fs.String("head-file", "", "Head report file")
	thresholds := thresholdFlag{}
	fs.Var(&thresholds, "threshold", "Threshold override key=value (repeatable)")
	if err := fs.Parse(args); err != nil {
		return ExitUsage, err
	}

	source := strings.TrimSpace(*baselineSource)
	if source == "" {
		return ExitUsage, fmt.Errorf("--baseline-source is required")
	}

	head, headRef, err := a.resolveReportSource(ctx, store, *headRunID, *headFile)
	if err != nil {
		return ExitConfigState, fmt.Errorf("resolve head report: %w", err)
	}
	base, err := report.LoadBaseline(report.BaselineOptions{Source: source, File: *baselineFile, URL: *baselineURL})
	if err != nil {
		return ExitConfigState, err
	}
	baseRef := source
	if *baselineFile != "" {
		baseRef = *baselineFile
	}
	if *baselineURL != "" {
		baseRef = *baselineURL
	}

	effectiveThresholds := mergeThresholdMaps(loaded.Config.CI.Thresholds, thresholds)
	cmp := report.Compare(base, head, effectiveThresholds, baseRef, headRef)

	if global.JSON {
		spec := map[string]any{"baselineSource": source, "base": baseRef, "head": headRef, "thresholds": effectiveThresholds}
		phase := "completed"
		if !cmp.Passed {
			phase = "failed"
		}
		resource := output.Resource{
			APIVersion: output.APIVersion,
			Kind:       "CIGateReport",
			Metadata:   output.ResourceMetadata{Command: "ci check", GeneratedAt: time.Now().UTC()},
			Spec:       spec,
			Status: output.ResourceStatus{
				Phase:   phase,
				Summary: map[string]any{"passed": cmp.Passed, "regressionCount": len(cmp.Regressions)},
				Result:  cmp,
			},
		}
		if !cmp.Passed {
			resource.Status.Errors = []output.ErrorItem{{Code: "regression", Message: strings.Join(cmp.Regressions, "; ")}}
		}
		if err := output.WriteJSON(a.io.Out, resource); err != nil {
			return ExitInternal, err
		}
	} else {
		if err := output.WriteCompareReport(a.io.Out, cmp); err != nil {
			return ExitInternal, err
		}
	}

	if !cmp.Passed {
		err := fmt.Errorf("ci regression check failed")
		if global.JSON {
			return ExitPolicyViolation, markReportedError(err)
		}
		return ExitPolicyViolation, err
	}
	return ExitOK, nil
}

func (a *App) runCIGitHubAction(global GlobalOptions, args []string) (int, error) {
	if len(args) == 0 || args[0] != "init" {
		return ExitUsage, fmt.Errorf("supported command: ci github-action init")
	}
	fs := flag.NewFlagSet("ci github-action init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	writePath := fs.String("write", "", "Write generated workflow to path")
	if err := fs.Parse(args[1:]); err != nil {
		return ExitUsage, err
	}

	template := strings.TrimSpace(`name: Buildgraph CI
on:
  pull_request:
  push:
    branches: [main]
jobs:
  buildgraph:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25.x"
      - run: go build ./cmd/buildgraph
      - run: ./buildgraph analyze run --json > buildgraph-report.json
      - run: ./buildgraph ci check --baseline-source ci-artifact --baseline-file buildgraph-baseline.json --head-file buildgraph-report.json
`) + "\n"

	if *writePath != "" {
		if err := os.MkdirAll(filepath.Dir(*writePath), 0o755); err != nil {
			return ExitConfigState, fmt.Errorf("create output directory: %w", err)
		}
		if err := os.WriteFile(*writePath, []byte(template), 0o644); err != nil {
			return ExitConfigState, fmt.Errorf("write template: %w", err)
		}
	}

	result := map[string]any{"provider": "github", "written": *writePath != "", "path": *writePath, "template": template}
	if global.JSON {
		if err := output.WriteJSON(a.io.Out, output.SuccessResource("CIGeneratorReport", "ci github-action init", nil, nil, result, 0)); err != nil {
			return ExitInternal, err
		}
	} else {
		if *writePath != "" {
			_, _ = fmt.Fprintf(a.io.Out, "Wrote GitHub Action template to %s\n", *writePath)
		} else {
			_, _ = fmt.Fprint(a.io.Out, template)
		}
	}
	return ExitOK, nil
}

func (a *App) runCIGitLab(global GlobalOptions, args []string) (int, error) {
	if len(args) == 0 || args[0] != "init" {
		return ExitUsage, fmt.Errorf("supported command: ci gitlab-ci init")
	}
	fs := flag.NewFlagSet("ci gitlab-ci init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	writePath := fs.String("write", "", "Write generated GitLab CI template to path")
	if err := fs.Parse(args[1:]); err != nil {
		return ExitUsage, err
	}

	template := strings.TrimSpace(`stages:
  - analyze
buildgraph_analyze:
  stage: analyze
  image: golang:1.25
  script:
    - go build ./cmd/buildgraph
    - ./buildgraph analyze run --json > buildgraph-report.json
    - ./buildgraph ci check --baseline-source ci-artifact --baseline-file buildgraph-baseline.json --head-file buildgraph-report.json
  artifacts:
    when: always
    paths:
      - buildgraph-report.json
`) + "\n"

	if *writePath != "" {
		if err := os.MkdirAll(filepath.Dir(*writePath), 0o755); err != nil {
			return ExitConfigState, fmt.Errorf("create output directory: %w", err)
		}
		if err := os.WriteFile(*writePath, []byte(template), 0o644); err != nil {
			return ExitConfigState, fmt.Errorf("write template: %w", err)
		}
	}

	result := map[string]any{"provider": "gitlab", "written": *writePath != "", "path": *writePath, "template": template}
	if global.JSON {
		if err := output.WriteJSON(a.io.Out, output.SuccessResource("CIGeneratorReport", "ci gitlab-ci init", nil, nil, result, 0)); err != nil {
			return ExitInternal, err
		}
	} else {
		if *writePath != "" {
			_, _ = fmt.Fprintf(a.io.Out, "Wrote GitLab CI template to %s\n", *writePath)
		} else {
			_, _ = fmt.Fprint(a.io.Out, template)
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
		return a.writeSimple(global.JSON, "auth login", "AuthReport", map[string]any{"status": "logged-in", "user": *user})
	case "logout":
		if err := manager.Delete(); err != nil {
			return ExitConfigState, err
		}
		return a.writeSimple(global.JSON, "auth logout", "AuthReport", map[string]any{"status": "logged-out"})
	case "whoami":
		creds, err := manager.Load()
		if err != nil {
			return ExitAuthDenied, fmt.Errorf("not logged in")
		}
		return a.writeSimple(global.JSON, "auth whoami", "AuthReport", map[string]any{"user": creds.User, "source": creds.Source, "storedAt": creds.StoredAt})
	default:
		return ExitUsage, fmt.Errorf("unsupported auth subcommand %q", subcommand)
	}
}

func (a *App) runConfig(global GlobalOptions, loaded config.Loaded, args []string) (int, error) {
	if len(args) == 0 || args[0] != "show" {
		return ExitUsage, fmt.Errorf("supported config command: show")
	}
	if global.JSON {
		return ExitOK, output.WriteJSON(a.io.Out, output.SuccessResource("ConfigReport", "config show", nil, nil, loaded, 0))
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
	return a.writeSimple(global.JSON, "version", "VersionReport", payload)
}

func (a *App) writeSimple(asJSON bool, command, kind string, result any) (int, error) {
	if asJSON {
		if err := output.WriteJSON(a.io.Out, output.SuccessResource(kind, command, nil, nil, result, 0)); err != nil {
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

func (a *App) resolveReportSource(ctx context.Context, store *state.Store, runID int64, file string) (report.BuildReport, string, error) {
	if runID > 0 {
		if store == nil {
			return report.BuildReport{}, "", fmt.Errorf("state store unavailable")
		}
		rec, err := store.GetReportByRunID(ctx, runID)
		if err != nil {
			return report.BuildReport{}, "", err
		}
		runReport, err := report.ParseBuildReportJSON([]byte(rec.ReportJSON))
		if err != nil {
			return report.BuildReport{}, "", err
		}
		runReport.RunID = rec.RunID
		return runReport, fmt.Sprintf("run:%d", runID), nil
	}
	if strings.TrimSpace(file) != "" {
		runReport, err := report.ReadBuildReportFile(file)
		if err != nil {
			return report.BuildReport{}, "", err
		}
		return runReport, file, nil
	}
	if store == nil {
		return report.BuildReport{}, "", fmt.Errorf("state store unavailable")
	}
	rec, err := store.GetLatestReport(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return report.BuildReport{}, "", fmt.Errorf("no reports available in state store")
		}
		return report.BuildReport{}, "", err
	}
	runReport, err := report.ParseBuildReportJSON([]byte(rec.ReportJSON))
	if err != nil {
		return report.BuildReport{}, "", err
	}
	runReport.RunID = rec.RunID
	return runReport, fmt.Sprintf("run:%d", rec.RunID), nil
}

func (a *App) resolveReportNamedSource(ctx context.Context, store *state.Store, source string) (report.BuildReport, string, error) {
	source = strings.TrimSpace(source)
	if strings.HasPrefix(source, "run:") {
		value := strings.TrimPrefix(source, "run:")
		runID, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return report.BuildReport{}, "", fmt.Errorf("invalid run source %q", source)
		}
		runReport, _, err := a.resolveReportSource(ctx, store, runID, "")
		return runReport, source, err
	}
	runReport, _, err := a.resolveReportSource(ctx, store, 0, source)
	return runReport, source, err
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

func normalizeProgressMode(value string, globalJSON bool) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(value))
	switch mode {
	case "", "auto":
		if globalJSON {
			return "none", nil
		}
		return "human", nil
	case "human", "json", "none":
		return mode, nil
	default:
		return "", fmt.Errorf("invalid progress mode %q", value)
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

type thresholdFlag map[string]float64

func (t *thresholdFlag) String() string {
	if t == nil {
		return ""
	}
	keys := make([]string, 0, len(*t))
	for key := range *t {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, (*t)[key]))
	}
	return strings.Join(parts, ",")
}

func (t *thresholdFlag) Set(value string) error {
	if *t == nil {
		*t = map[string]float64{}
	}
	parts := strings.SplitN(strings.TrimSpace(value), "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("threshold must be key=value")
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return fmt.Errorf("invalid threshold value %q", parts[1])
	}
	(*t)[strings.TrimSpace(parts[0])] = parsed
	return nil
}

func mergeThresholdMaps(base map[string]float64, overrides map[string]float64) map[string]float64 {
	result := map[string]float64{}
	for key, value := range report.DefaultThresholds() {
		result[key] = value
	}
	for key, value := range base {
		result[key] = value
	}
	for key, value := range overrides {
		result[key] = value
	}
	return result
}

func (a *App) writeError(asJSON bool, command string, err error) {
	if err == nil {
		return
	}
	if asJSON {
		resource := output.ErrorResource("ErrorReport", command, nil, nil, []output.ErrorItem{{Code: "error", Message: err.Error()}}, 0)
		_ = output.WriteJSON(a.io.Err, resource)
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
	fmt.Fprintln(w, "  analyze              Analyze Dockerfile and build context")
	fmt.Fprintln(w, "  analyze run          Execute build and emit BuildReport")
	fmt.Fprintln(w, "  build                Execute BuildKit build")
	fmt.Fprintln(w, "  report show          Show BuildReport")
	fmt.Fprintln(w, "  report metrics       Show BuildReport metrics")
	fmt.Fprintln(w, "  report compare       Compare BuildReports")
	fmt.Fprintln(w, "  report trend         Show trend across recent BuildReports")
	fmt.Fprintln(w, "  report export        Export BuildReport graph to DOT/SVG")
	fmt.Fprintln(w, "  ci check             Evaluate CI regression policy")
	fmt.Fprintln(w, "  ci github-action     Generate GitHub Action template")
	fmt.Fprintln(w, "  ci gitlab-ci         Generate GitLab CI template")
	fmt.Fprintln(w, "  backend list         List available backends")
	fmt.Fprintln(w, "  doctor               Run environment diagnostics")
	fmt.Fprintln(w, "  auth                 Manage SaaS authentication state")
	fmt.Fprintln(w, "  config show          Show effective config")
	fmt.Fprintln(w, "  version              Print version information")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Global Flags:")
	fmt.Fprintln(w, "  --json                 Render JSON output (buildgraph.dev/v2)")
	fmt.Fprintln(w, "  --no-color             Disable color")
	fmt.Fprintln(w, "  --verbose              Verbose mode")
	fmt.Fprintln(w, "  --config PATH          Global config path")
	fmt.Fprintln(w, "  --project-config PATH  Project config path")
	fmt.Fprintln(w, "  --profile NAME         Config profile")
	fmt.Fprintln(w, "  --non-interactive      Disable prompts")
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
