package report

import "testing"

func TestCompareDetectsRegressions(t *testing.T) {
	t.Parallel()
	base := BuildReport{}
	base.Summary.DurationMS = 100
	base.Summary.CacheMisses = 10
	base.Summary.WarningCount = 0
	base.Metrics.CriticalPathMS = 80
	base.Metrics.CacheHitRatio = 0.9

	head := BuildReport{}
	head.Summary.DurationMS = 130
	head.Summary.CacheMisses = 15
	head.Summary.WarningCount = 1
	head.Metrics.CriticalPathMS = 100
	head.Metrics.CacheHitRatio = 0.7

	cmp := Compare(base, head, DefaultThresholds(), "base", "head")
	if cmp.Passed {
		t.Fatalf("expected compare to fail")
	}
	if len(cmp.Regressions) == 0 {
		t.Fatalf("expected regressions")
	}
}
