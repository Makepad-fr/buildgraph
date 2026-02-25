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

func WriteDoctor(w io.Writer, checks map[string]string) error {
	keys := make([]string, 0, len(checks))
	for key := range checks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, err := fmt.Fprintf(w, "%s: %s\n", key, checks[key]); err != nil {
			return err
		}
	}
	return nil
}
