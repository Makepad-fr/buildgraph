package output

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Makepad-fr/buildgraph/internal/report"
)

func WriteBuildReport(w io.Writer, run report.BuildReport) error {
	if _, err := fmt.Fprintf(w, "Report generated: %s\n", run.GeneratedAt.Format("2006-01-02T15:04:05Z07:00")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Command: %s\nBackend: %s\nEndpoint: %s\n", run.Command, run.Backend, run.Endpoint); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Duration: %dms\nCritical path: %dms\nCache hit ratio: %.2f%%\n", run.Summary.DurationMS, run.Metrics.CriticalPathMS, run.Metrics.CacheHitRatio*100); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Graph: %s (%d vertices, %d edges)\n", run.GraphCompleteness, run.Summary.VertexCount, run.Summary.EdgeCount); err != nil {
		return err
	}
	if len(run.Metrics.TopSlowVertices) > 0 {
		if _, err := fmt.Fprintln(w, "Top slow vertices:"); err != nil {
			return err
		}
		for _, vertex := range run.Metrics.TopSlowVertices {
			if _, err := fmt.Fprintf(w, "- %s (%dms)\n", strings.TrimSpace(vertex.Name), vertex.DurationMS); err != nil {
				return err
			}
		}
	}
	if len(run.Findings) > 0 {
		if _, err := fmt.Fprintf(w, "Findings: %d\n", len(run.Findings)); err != nil {
			return err
		}
	}
	return nil
}

func WriteCompareReport(w io.Writer, cmp report.CompareReport) error {
	status := "PASS"
	if !cmp.Passed {
		status = "FAIL"
	}
	if _, err := fmt.Fprintf(w, "Compare: %s -> %s\nStatus: %s\n", cmp.BaseRef, cmp.HeadRef, status); err != nil {
		return err
	}
	for _, metric := range cmp.Metrics {
		marker := "ok"
		if metric.Breached {
			marker = "regression"
		}
		if _, err := fmt.Fprintf(w, "- %s: base=%.2f head=%.2f delta=%.2f%% threshold=%.2f [%s]\n", metric.Key, metric.Base, metric.Head, metric.DeltaPct, metric.Threshold, marker); err != nil {
			return err
		}
	}
	if len(cmp.Regressions) > 0 {
		if _, err := fmt.Fprintln(w, "Regressions:"); err != nil {
			return err
		}
		for _, msg := range cmp.Regressions {
			if _, err := fmt.Fprintf(w, "- %s\n", msg); err != nil {
				return err
			}
		}
	}
	return nil
}

func WriteTrendReport(w io.Writer, trend report.TrendReport) error {
	if _, err := fmt.Fprintf(w, "Trend window: %d runs\n", trend.Window); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Average duration: %.2fms\nAverage critical path: %.2fms\nAverage cache hit ratio: %.2f%%\n", trend.AverageDurationMS, trend.AverageCriticalMS, trend.AverageCacheRatio*100); err != nil {
		return err
	}
	if len(trend.Signals) > 0 {
		signals := append([]string(nil), trend.Signals...)
		sort.Strings(signals)
		if _, err := fmt.Fprintln(w, "Signals:"); err != nil {
			return err
		}
		for _, signal := range signals {
			if _, err := fmt.Fprintf(w, "- %s\n", signal); err != nil {
				return err
			}
		}
	}
	return nil
}
