package report

import (
	"sort"
	"time"

	"github.com/Makepad-fr/buildgraph/internal/analyze"
	"github.com/Makepad-fr/buildgraph/internal/backend"
)

type BuildSummary struct {
	DurationMS         int64          `json:"durationMs"`
	CacheHits          int            `json:"cacheHits"`
	CacheMisses        int            `json:"cacheMisses"`
	WarningCount       int            `json:"warningCount"`
	FindingCount       int            `json:"findingCount"`
	FindingsBySeverity map[string]int `json:"findingsBySeverity"`
	VertexCount        int            `json:"vertexCount"`
	EdgeCount          int            `json:"edgeCount"`
}

type BuildReport struct {
	RunID             int64                `json:"runId,omitempty"`
	GeneratedAt       time.Time            `json:"generatedAt"`
	Command           string               `json:"command"`
	ContextDir        string               `json:"contextDir,omitempty"`
	Dockerfile        string               `json:"dockerfile,omitempty"`
	Backend           string               `json:"backend"`
	Endpoint          string               `json:"endpoint"`
	GraphCompleteness string               `json:"graphCompleteness"`
	Build             backend.BuildResult  `json:"build"`
	Findings          []backend.Finding    `json:"findings"`
	StageGraph        analyze.StageGraph   `json:"stageGraph"`
	Metrics           backend.BuildMetrics `json:"metrics"`
	Summary           BuildSummary         `json:"summary"`
}

type BuildReportInput struct {
	RunID       int64
	Command     string
	ContextDir  string
	Dockerfile  string
	Build       backend.BuildResult
	Findings    []backend.Finding
	StageGraph  analyze.StageGraph
	GeneratedAt time.Time
}

func NewBuildReport(input BuildReportInput) BuildReport {
	generatedAt := input.GeneratedAt.UTC()
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}
	report := BuildReport{
		RunID:       input.RunID,
		GeneratedAt: generatedAt,
		Command:     input.Command,
		ContextDir:  input.ContextDir,
		Dockerfile:  input.Dockerfile,
		Backend:     input.Build.Backend,
		Endpoint:    input.Build.Endpoint,
		Build:       input.Build,
		Findings:    append([]backend.Finding(nil), input.Findings...),
		StageGraph:  input.StageGraph,
		Metrics:     ComputeBuildMetrics(input.Build.Vertices, input.Build.Edges, input.Build.CacheStats),
	}
	report.Summary = buildSummary(report)
	report.GraphCompleteness = graphCompleteness(input.Build)
	sortFindings(report.Findings)
	return report
}

func graphCompleteness(result backend.BuildResult) string {
	if len(result.Vertices) == 0 {
		return "none"
	}
	if result.GraphComplete {
		return "complete"
	}
	return "partial"
}

func sortFindings(findings []backend.Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		left := backend.SeverityRank(findings[i].Severity)
		right := backend.SeverityRank(findings[j].Severity)
		if left == right {
			if findings[i].Dimension == findings[j].Dimension {
				if findings[i].File == findings[j].File {
					return findings[i].Line < findings[j].Line
				}
				return findings[i].File < findings[j].File
			}
			return findings[i].Dimension < findings[j].Dimension
		}
		return left > right
	})
}

func buildSummary(report BuildReport) BuildSummary {
	findingsBySeverity := map[string]int{}
	for _, finding := range report.Findings {
		findingsBySeverity[finding.Severity]++
	}
	return BuildSummary{
		DurationMS:         report.Metrics.TimeDistribution["totalMs"],
		CacheHits:          report.Build.CacheStats.Hits,
		CacheMisses:        report.Build.CacheStats.Misses,
		WarningCount:       len(report.Build.Warnings),
		FindingCount:       len(report.Findings),
		FindingsBySeverity: findingsBySeverity,
		VertexCount:        len(report.Build.Vertices),
		EdgeCount:          len(report.Build.Edges),
	}
}
