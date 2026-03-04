package backend

import (
	"context"
	"errors"
	"time"
)

const (
	DimensionPerformance     = "performance"
	DimensionCacheability    = "cacheability"
	DimensionReproducibility = "reproducibility"
	DimensionSecurity        = "security"
	DimensionPolicy          = "policy"
)

const (
	SeverityLow      = "low"
	SeverityMedium   = "medium"
	SeverityHigh     = "high"
	SeverityCritical = "critical"
)

const (
	OutputImage = "image"
	OutputOCI   = "oci"
	OutputLocal = "local"
)

var (
	ErrBackendUnavailable = errors.New("backend unavailable")
	ErrBuildFailed        = errors.New("build failed")
)

type Finding struct {
	ID         string `json:"id"`
	Dimension  string `json:"dimension"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Suggestion string `json:"suggestion"`
	DocsURL    string `json:"docsUrl"`
}

type AnalyzeRequest struct {
	ContextDir         string
	Dockerfile         string
	SeverityThreshold  string
	FailOn             string
	Backend            string
	Endpoint           string
	ProjectConfigPath  string
	GlobalConfigPath   string
	EnablePolicyChecks bool
}

type AnalyzeResult struct {
	Backend  string    `json:"backend"`
	Endpoint string    `json:"endpoint"`
	Findings []Finding `json:"findings"`
}

type BuildArg struct {
	Key   string
	Value string
}

type SecretSpec struct {
	ID  string
	Src string
}

type BuildRequest struct {
	ContextDir string
	Dockerfile string
	Target     string
	Platforms  []string
	BuildArgs  []BuildArg
	Secrets    []SecretSpec

	OutputMode string
	ImageRef   string
	OCIDest    string
	LocalDest  string

	Backend           string
	Endpoint          string
	ProjectConfigPath string
	GlobalConfigPath  string
}

type CacheStats struct {
	Hits   int `json:"hits"`
	Misses int `json:"misses"`
}

type BuildVertex struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Stage       string     `json:"stage,omitempty"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	DurationMS  int64      `json:"durationMs"`
	Cached      bool       `json:"cached"`
}

type BuildEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type SlowVertex struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	DurationMS int64  `json:"durationMs"`
}

type BuildMetrics struct {
	CriticalPathMS       int64            `json:"criticalPathMs"`
	CriticalPathVertices []string         `json:"criticalPathVertices"`
	CacheHitRatio        float64          `json:"cacheHitRatio"`
	StageDistribution    map[string]int64 `json:"stageDistribution"`
	TimeDistribution     map[string]int64 `json:"timeDistribution"`
	LongestChain         int              `json:"longestChain"`
	TopSlowVertices      []SlowVertex     `json:"topSlowVertices,omitempty"`
	RepeatedMissPatterns []string         `json:"repeatedMissPatterns,omitempty"`
}

type BuildResult struct {
	Backend             string            `json:"backend"`
	Endpoint            string            `json:"endpoint"`
	Outputs             []string          `json:"outputs"`
	Digest              string            `json:"digest"`
	ProvenanceAvailable bool              `json:"provenanceAvailable"`
	CacheStats          CacheStats        `json:"cacheStats"`
	Vertices            []BuildVertex     `json:"vertices,omitempty"`
	Edges               []BuildEdge       `json:"edges,omitempty"`
	GraphComplete       bool              `json:"graphComplete"`
	Warnings            []string          `json:"warnings"`
	ExporterResponse    map[string]string `json:"-"`
}

type DetectRequest struct {
	Backend           string
	Endpoint          string
	ProjectConfigPath string
	GlobalConfigPath  string
}

type DetectResult struct {
	Backend   string            `json:"backend"`
	Endpoint  string            `json:"endpoint"`
	Mode      string            `json:"mode"`
	Available bool              `json:"available"`
	Details   string            `json:"details"`
	Attempts  []DetectAttempt   `json:"attempts,omitempty"`
	Metadata  map[string]string `json:"metadata"`
}

type DetectAttempt struct {
	Source   string `json:"source"`
	Endpoint string `json:"endpoint"`
	Mode     string `json:"mode"`
	Status   string `json:"status"`
	Details  string `json:"details,omitempty"`
	Error    string `json:"error,omitempty"`
}

type BackendCapabilities struct {
	SupportsAnalyze       bool `json:"supportsAnalyze"`
	SupportsImageOutput   bool `json:"supportsImageOutput"`
	SupportsOCIOutput     bool `json:"supportsOCIOutput"`
	SupportsLocalOutput   bool `json:"supportsLocalOutput"`
	SupportsRemoteBuild   bool `json:"supportsRemoteBuild"`
	SupportsProgressEvent bool `json:"supportsProgressEvent"`
}

type BuildProgressEvent struct {
	Timestamp time.Time  `json:"timestamp"`
	Phase     string     `json:"phase"`
	Message   string     `json:"message"`
	VertexID  string     `json:"vertexId,omitempty"`
	Inputs    []string   `json:"inputs,omitempty"`
	Status    string     `json:"status,omitempty"`
	Started   *time.Time `json:"started,omitempty"`
	Completed *time.Time `json:"completed,omitempty"`
	Cached    bool       `json:"cached,omitempty"`
	Error     string     `json:"error,omitempty"`
}

type BuildProgressFunc func(BuildProgressEvent)

type Backend interface {
	Name() string
	Detect(ctx context.Context, req DetectRequest) (DetectResult, error)
	Analyze(ctx context.Context, req AnalyzeRequest) (AnalyzeResult, error)
	Build(ctx context.Context, req BuildRequest, progress BuildProgressFunc) (BuildResult, error)
	Capabilities(ctx context.Context) (BackendCapabilities, error)
}

func SeverityRank(value string) int {
	switch value {
	case SeverityLow:
		return 1
	case SeverityMedium:
		return 2
	case SeverityHigh:
		return 3
	case SeverityCritical:
		return 4
	default:
		return 0
	}
}

func FindingMeetsThreshold(f Finding, threshold string) bool {
	if threshold == "" {
		return true
	}
	return SeverityRank(f.Severity) >= SeverityRank(threshold)
}
