package analyze

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Makepad-fr/buildgraph/internal/backend"
)

type Engine struct {
	rules map[string]ruleTemplate
}

type ruleTemplate struct {
	ID         string
	Dimension  string
	Severity   string
	Message    string
	Suggestion string
	DocsURL    string
}

func NewEngine() *Engine {
	rules := BuiltinRules()
	templates := make(map[string]ruleTemplate, len(rules))
	for id, rule := range rules {
		templates[id] = ruleTemplate{
			ID:         rule.ID,
			Dimension:  rule.Dimension,
			Severity:   rule.Severity,
			Message:    rule.Message,
			Suggestion: rule.Suggestion,
			DocsURL:    rule.DocsRef,
		}
	}
	return &Engine{rules: templates}
}

func (e *Engine) Analyze(ctx context.Context, req backend.AnalyzeRequest) ([]backend.Finding, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	parsed, err := ParseDockerfile(req.ContextDir, req.Dockerfile)
	if err != nil {
		return nil, err
	}

	findings := make([]backend.Finding, 0, 16)
	appendFinding := func(ruleID string, line int, extra string) {
		tpl, ok := e.rules[ruleID]
		if !ok {
			return
		}
		message := tpl.Message
		if extra != "" {
			message = fmt.Sprintf("%s %s", tpl.Message, extra)
		}
		findings = append(findings, backend.Finding{
			ID:         tpl.ID,
			Dimension:  tpl.Dimension,
			Severity:   tpl.Severity,
			Message:    message,
			File:       filepath.Base(parsed.Path),
			Line:       line,
			Suggestion: tpl.Suggestion,
			DocsURL:    tpl.DocsURL,
		})
	}

	runCount := 0
	hasUser := false
	hasHealthcheck := false
	hasSourceLabel := false
	firstHeavyRunLine := 0
	argAfterHeavyRun := false
	seenCopyAllBeforeInstall := false

	secretEnvPattern := regexp.MustCompile(`(?i)(secret|token|password|passwd|apikey|api_key)`)
	curlPipePattern := regexp.MustCompile(`(?i)(curl|wget)[^|]+\|\s*(sh|bash)`)

	for _, inst := range parsed.Instructions {
		cmd := inst.Command
		valueLower := strings.ToLower(inst.Value)

		switch cmd {
		case "RUN":
			runCount++
			if firstHeavyRunLine == 0 && (strings.Contains(valueLower, "apt-get") || strings.Contains(valueLower, "npm install") || strings.Contains(valueLower, "go mod download")) {
				firstHeavyRunLine = inst.Line
			}
			if strings.Contains(valueLower, "apt-get update") && !strings.Contains(valueLower, "apt-get install") {
				appendFinding("BG_PERF_APT_SPLIT", inst.Line, "")
			}
			if strings.Contains(valueLower, "apt-get install") && !strings.Contains(valueLower, "=") {
				appendFinding("BG_REPRO_APT_UNPINNED", inst.Line, "")
			}
			if curlPipePattern.MatchString(valueLower) {
				appendFinding("BG_SEC_CURL_PIPE_SH", inst.Line, "")
			}
		case "COPY", "ADD":
			if strings.Contains(valueLower, " .") || strings.HasPrefix(valueLower, ". ") || strings.HasPrefix(valueLower, "./ ") {
				if firstHeavyRunLine == 0 {
					seenCopyAllBeforeInstall = true
					appendFinding("BG_CACHE_COPY_ALL_EARLY", inst.Line, "")
				}
			}
		case "ARG":
			if firstHeavyRunLine > 0 {
				argAfterHeavyRun = true
				appendFinding("BG_CACHE_ARG_LATE", inst.Line, "")
			}
		case "FROM":
			if strings.Contains(valueLower, ":latest") || (!strings.Contains(valueLower, "@sha256:") && !strings.Contains(valueLower, ":")) {
				appendFinding("BG_REPRO_FROM_MUTABLE", inst.Line, "")
			}
		case "USER":
			hasUser = true
			if strings.TrimSpace(valueLower) == "root" || strings.HasPrefix(strings.TrimSpace(valueLower), "0") {
				appendFinding("BG_SEC_ROOT_USER", inst.Line, "")
			}
		case "ENV":
			if secretEnvPattern.MatchString(inst.Value) {
				appendFinding("BG_SEC_PLAIN_SECRET_ENV", inst.Line, "")
			}
		case "LABEL":
			if strings.Contains(valueLower, "org.opencontainers.image.source") {
				hasSourceLabel = true
			}
		case "HEALTHCHECK":
			hasHealthcheck = true
		}
	}

	if runCount > 5 {
		appendFinding("BG_PERF_TOO_MANY_RUN", 1, fmt.Sprintf("Detected %d RUN instructions.", runCount))
	}
	if !hasUser {
		appendFinding("BG_SEC_ROOT_USER", 1, "No USER instruction found, container defaults to root.")
	}
	if !hasSourceLabel {
		appendFinding("BG_POL_MISSING_SOURCE_LABEL", 1, "")
	}
	if !hasHealthcheck {
		appendFinding("BG_POL_MISSING_HEALTHCHECK", 1, "")
	}

	_ = argAfterHeavyRun
	_ = seenCopyAllBeforeInstall

	return findings, nil
}
