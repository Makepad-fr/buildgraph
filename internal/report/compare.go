package report

import (
	"fmt"
	"sort"
)

const (
	ThresholdDurationTotalPct  = "duration_total_pct"
	ThresholdCriticalPathPct   = "critical_path_pct"
	ThresholdCacheHitRatioDrop = "cache_hit_ratio_pp_drop"
	ThresholdCacheMissCountPct = "cache_miss_count_pct"
	ThresholdWarningCountDelta = "warning_count_delta"
)

func DefaultThresholds() map[string]float64 {
	return map[string]float64{
		ThresholdDurationTotalPct:  10,
		ThresholdCriticalPathPct:   10,
		ThresholdCacheHitRatioDrop: 10,
		ThresholdCacheMissCountPct: 15,
		ThresholdWarningCountDelta: 0,
	}
}

type MetricDelta struct {
	Key       string  `json:"key"`
	Base      float64 `json:"base"`
	Head      float64 `json:"head"`
	Delta     float64 `json:"delta"`
	DeltaPct  float64 `json:"deltaPct"`
	Threshold float64 `json:"threshold"`
	Breached  bool    `json:"breached"`
	Message   string  `json:"message"`
}

type CompareReport struct {
	BaseRef     string        `json:"baseRef"`
	HeadRef     string        `json:"headRef"`
	Metrics     []MetricDelta `json:"metrics"`
	Regressions []string      `json:"regressions"`
	Passed      bool          `json:"passed"`
}

func Compare(base, head BuildReport, thresholds map[string]float64, baseRef, headRef string) CompareReport {
	effective := mergeThresholds(thresholds)
	deltas := make([]MetricDelta, 0, 5)

	deltas = append(deltas, evaluateIncreaseMetric(
		ThresholdDurationTotalPct,
		float64(base.Summary.DurationMS),
		float64(head.Summary.DurationMS),
		effective[ThresholdDurationTotalPct],
		"total duration",
	))
	deltas = append(deltas, evaluateIncreaseMetric(
		ThresholdCriticalPathPct,
		float64(base.Metrics.CriticalPathMS),
		float64(head.Metrics.CriticalPathMS),
		effective[ThresholdCriticalPathPct],
		"critical path",
	))
	deltas = append(deltas, evaluateDecreaseMetric(
		ThresholdCacheHitRatioDrop,
		base.Metrics.CacheHitRatio*100,
		head.Metrics.CacheHitRatio*100,
		effective[ThresholdCacheHitRatioDrop],
		"cache hit ratio (percentage points)",
	))
	deltas = append(deltas, evaluateIncreaseMetric(
		ThresholdCacheMissCountPct,
		float64(base.Summary.CacheMisses),
		float64(head.Summary.CacheMisses),
		effective[ThresholdCacheMissCountPct],
		"cache misses",
	))
	deltas = append(deltas, evaluateAbsoluteIncrease(
		ThresholdWarningCountDelta,
		float64(base.Summary.WarningCount),
		float64(head.Summary.WarningCount),
		effective[ThresholdWarningCountDelta],
		"warnings",
	))

	regressions := make([]string, 0)
	for _, metric := range deltas {
		if metric.Breached {
			regressions = append(regressions, metric.Message)
		}
	}
	sort.Strings(regressions)

	return CompareReport{
		BaseRef:     baseRef,
		HeadRef:     headRef,
		Metrics:     deltas,
		Regressions: regressions,
		Passed:      len(regressions) == 0,
	}
}

func mergeThresholds(overrides map[string]float64) map[string]float64 {
	effective := DefaultThresholds()
	for key, value := range overrides {
		effective[key] = value
	}
	return effective
}

func evaluateIncreaseMetric(key string, base, head, threshold float64, label string) MetricDelta {
	delta := head - base
	deltaPct := percentDelta(base, head)
	breached := deltaPct > threshold
	return MetricDelta{
		Key:       key,
		Base:      base,
		Head:      head,
		Delta:     delta,
		DeltaPct:  deltaPct,
		Threshold: threshold,
		Breached:  breached,
		Message:   fmt.Sprintf("%s regression %.2f%% exceeds %.2f%% threshold", label, deltaPct, threshold),
	}
}

func evaluateDecreaseMetric(key string, base, head, threshold float64, label string) MetricDelta {
	drop := base - head
	breached := drop > threshold
	return MetricDelta{
		Key:       key,
		Base:      base,
		Head:      head,
		Delta:     head - base,
		DeltaPct:  drop,
		Threshold: threshold,
		Breached:  breached,
		Message:   fmt.Sprintf("%s dropped %.2f and exceeds %.2f threshold", label, drop, threshold),
	}
}

func evaluateAbsoluteIncrease(key string, base, head, threshold float64, label string) MetricDelta {
	delta := head - base
	breached := delta > threshold
	return MetricDelta{
		Key:       key,
		Base:      base,
		Head:      head,
		Delta:     delta,
		DeltaPct:  delta,
		Threshold: threshold,
		Breached:  breached,
		Message:   fmt.Sprintf("%s increased by %.2f and exceeds %.2f threshold", label, delta, threshold),
	}
}

func percentDelta(base, head float64) float64 {
	if base == 0 {
		if head == 0 {
			return 0
		}
		return 100
	}
	return ((head - base) / base) * 100
}
