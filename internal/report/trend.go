package report

import "time"

type TrendPoint struct {
	RunID             int64     `json:"runId,omitempty"`
	GeneratedAt       time.Time `json:"generatedAt"`
	DurationMS        int64     `json:"durationMs"`
	CriticalPathMS    int64     `json:"criticalPathMs"`
	CacheHitRatio     float64   `json:"cacheHitRatio"`
	WarningCount      int       `json:"warningCount"`
	GraphCompleteness string    `json:"graphCompleteness"`
}

type TrendReport struct {
	Window            int          `json:"window"`
	AverageDurationMS float64      `json:"averageDurationMs"`
	AverageCriticalMS float64      `json:"averageCriticalPathMs"`
	AverageCacheRatio float64      `json:"averageCacheHitRatio"`
	Points            []TrendPoint `json:"points"`
	Signals           []string     `json:"signals"`
}

func BuildTrend(reports []BuildReport) TrendReport {
	trend := TrendReport{
		Window:  len(reports),
		Points:  make([]TrendPoint, 0, len(reports)),
		Signals: make([]string, 0, 3),
	}
	if len(reports) == 0 {
		return trend
	}

	var durationTotal int64
	var criticalTotal int64
	var cacheRatioTotal float64
	for _, run := range reports {
		trend.Points = append(trend.Points, TrendPoint{
			RunID:             run.RunID,
			GeneratedAt:       run.GeneratedAt,
			DurationMS:        run.Summary.DurationMS,
			CriticalPathMS:    run.Metrics.CriticalPathMS,
			CacheHitRatio:     run.Metrics.CacheHitRatio,
			WarningCount:      run.Summary.WarningCount,
			GraphCompleteness: run.GraphCompleteness,
		})
		durationTotal += run.Summary.DurationMS
		criticalTotal += run.Metrics.CriticalPathMS
		cacheRatioTotal += run.Metrics.CacheHitRatio
	}

	count := float64(len(reports))
	trend.AverageDurationMS = float64(durationTotal) / count
	trend.AverageCriticalMS = float64(criticalTotal) / count
	trend.AverageCacheRatio = cacheRatioTotal / count

	if len(reports) >= 2 {
		last := reports[len(reports)-1]
		prev := reports[len(reports)-2]
		if last.Summary.DurationMS > prev.Summary.DurationMS {
			trend.Signals = append(trend.Signals, "latest run is slower than previous run")
		}
		if last.Metrics.CacheHitRatio < prev.Metrics.CacheHitRatio {
			trend.Signals = append(trend.Signals, "latest cache hit ratio dropped")
		}
		if last.Metrics.CriticalPathMS > prev.Metrics.CriticalPathMS {
			trend.Signals = append(trend.Signals, "latest critical path increased")
		}
	}

	return trend
}
