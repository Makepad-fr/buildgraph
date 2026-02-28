package output

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Makepad-fr/buildgraph/internal/backend"
)

func WriteAnalyze(w io.Writer, result backend.AnalyzeResult) error {
	if _, err := fmt.Fprintf(w, "Backend: %s\nEndpoint: %s\n\n", result.Backend, result.Endpoint); err != nil {
		return err
	}
	if len(result.Findings) == 0 {
		_, err := fmt.Fprintln(w, "No findings.")
		return err
	}
	sort.SliceStable(result.Findings, func(i, j int) bool {
		return backend.SeverityRank(result.Findings[i].Severity) > backend.SeverityRank(result.Findings[j].Severity)
	})
	for _, finding := range result.Findings {
		if _, err := fmt.Fprintf(w, "- [%s][%s] %s (%s:%d)\n  Suggestion: %s\n  Docs: %s\n",
			strings.ToUpper(finding.Severity),
			finding.Dimension,
			finding.Message,
			finding.File,
			finding.Line,
			finding.Suggestion,
			finding.DocsURL,
		); err != nil {
			return err
		}
	}
	return nil
}

func WriteBuild(w io.Writer, result backend.BuildResult) error {
	if _, err := fmt.Fprintf(w, "Backend: %s\nEndpoint: %s\n", result.Backend, result.Endpoint); err != nil {
		return err
	}
	if result.Digest != "" {
		if _, err := fmt.Fprintf(w, "Digest: %s\n", result.Digest); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "Outputs: %s\n", strings.Join(result.Outputs, ", ")); err != nil {
		return err
	}
	if len(result.Warnings) > 0 {
		if _, err := fmt.Fprintln(w, "Warnings:"); err != nil {
			return err
		}
		for _, warning := range result.Warnings {
			if _, err := fmt.Fprintf(w, "- %s\n", warning); err != nil {
				return err
			}
		}
	}
	return nil
}

type DoctorReport struct {
	Checks        map[string]string       `json:"checks"`
	Attempts      []backend.DetectAttempt `json:"attempts"`
	Found         backend.DetectResult    `json:"found"`
	ConfigSnippet string                  `json:"configSnippet"`
	CommonFixes   []string                `json:"commonFixes"`
}

func WriteDoctor(w io.Writer, report DoctorReport) error {
	keys := make([]string, 0, len(report.Checks))
	for key := range report.Checks {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	if _, err := fmt.Fprintln(w, "Checks:"); err != nil {
		return err
	}
	for _, key := range keys {
		if _, err := fmt.Fprintf(w, "- %s: %s\n", key, report.Checks[key]); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(w, "\nWhat it tried:"); err != nil {
		return err
	}
	if len(report.Attempts) == 0 {
		if _, err := fmt.Fprintln(w, "- (no backend detection attempts recorded)"); err != nil {
			return err
		}
	} else {
		for i, attempt := range report.Attempts {
			line := fmt.Sprintf("%d. [%s] source=%s mode=%s endpoint=%s", i+1, strings.ToUpper(attempt.Status), attempt.Source, attempt.Mode, attempt.Endpoint)
			if _, err := fmt.Fprintln(w, line); err != nil {
				return err
			}
			if attempt.Details != "" {
				if _, err := fmt.Fprintf(w, "   details: %s\n", attempt.Details); err != nil {
					return err
				}
			}
			if attempt.Error != "" {
				if _, err := fmt.Fprintf(w, "   error: %s\n", attempt.Error); err != nil {
					return err
				}
			}
		}
	}

	if _, err := fmt.Fprintln(w, "\nWhat it found:"); err != nil {
		return err
	}
	if report.Found.Available {
		if _, err := fmt.Fprintf(w, "- backend: %s\n- mode: %s\n- endpoint: %s\n- details: %s\n", report.Found.Backend, report.Found.Mode, report.Found.Endpoint, report.Found.Details); err != nil {
			return err
		}
		if source := report.Found.Metadata["resolutionSource"]; source != "" {
			if _, err := fmt.Fprintf(w, "- resolution source: %s\n", source); err != nil {
				return err
			}
		}
	} else {
		if _, err := fmt.Fprintf(w, "- backend detection failed: %s\n", report.Found.Details); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(w, "\nPaste into .buildgraph.yaml:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, report.ConfigSnippet); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w, "Common fixes:"); err != nil {
		return err
	}
	for _, fix := range report.CommonFixes {
		if _, err := fmt.Fprintf(w, "- %s\n", fix); err != nil {
			return err
		}
	}

	return nil
}
