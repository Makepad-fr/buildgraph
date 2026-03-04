package report

import (
	"sort"
	"strings"

	"github.com/Makepad-fr/buildgraph/internal/backend"
)

func ComputeBuildMetrics(vertices []backend.BuildVertex, edges []backend.BuildEdge, cache backend.CacheStats) backend.BuildMetrics {
	metrics := backend.BuildMetrics{
		StageDistribution: map[string]int64{},
		TimeDistribution:  map[string]int64{},
	}
	if len(vertices) == 0 {
		return metrics
	}

	vertexByID := map[string]backend.BuildVertex{}
	adj := map[string][]string{}
	indegree := map[string]int{}

	var totalMs int64
	var cachedMs int64
	var uncachedMs int64
	missPatternCount := map[string]int{}
	slow := make([]backend.SlowVertex, 0, len(vertices))

	for _, vertex := range vertices {
		duration := vertex.DurationMS
		if duration < 0 {
			duration = 0
		}
		vertex.DurationMS = duration
		vertexByID[vertex.ID] = vertex
		if _, ok := indegree[vertex.ID]; !ok {
			indegree[vertex.ID] = 0
		}

		totalMs += duration
		stage := strings.TrimSpace(vertex.Stage)
		if stage == "" {
			stage = "unknown"
		}
		metrics.StageDistribution[stage] += duration
		if vertex.Cached {
			cachedMs += duration
		} else {
			uncachedMs += duration
			norm := normalizePattern(vertex.Name)
			if norm != "" {
				missPatternCount[norm]++
			}
		}
		slow = append(slow, backend.SlowVertex{ID: vertex.ID, Name: vertex.Name, DurationMS: duration})
	}

	for _, edge := range edges {
		if edge.From == "" || edge.To == "" || edge.From == edge.To {
			continue
		}
		if _, ok := vertexByID[edge.From]; !ok {
			continue
		}
		if _, ok := vertexByID[edge.To]; !ok {
			continue
		}
		adj[edge.From] = append(adj[edge.From], edge.To)
		indegree[edge.To]++
	}

	queue := make([]string, 0, len(indegree))
	for id, degree := range indegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue)
	topo := make([]string, 0, len(indegree))

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		topo = append(topo, id)
		for _, next := range adj[id] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
			}
		}
		sort.Strings(queue)
	}

	if len(topo) == len(vertexByID) {
		metrics.CriticalPathMS, metrics.CriticalPathVertices, metrics.LongestChain = longestPath(topo, vertexByID, adj)
	} else {
		// Cycles are not expected, but prefer deterministic fallback over failing report generation.
		for _, candidate := range slow {
			if candidate.DurationMS > metrics.CriticalPathMS {
				metrics.CriticalPathMS = candidate.DurationMS
				metrics.CriticalPathVertices = []string{candidate.ID}
				metrics.LongestChain = 1
			}
		}
	}

	totalCache := cache.Hits + cache.Misses
	if totalCache > 0 {
		metrics.CacheHitRatio = float64(cache.Hits) / float64(totalCache)
	}

	metrics.TimeDistribution["totalMs"] = totalMs
	metrics.TimeDistribution["cachedMs"] = cachedMs
	metrics.TimeDistribution["uncachedMs"] = uncachedMs

	sort.Slice(slow, func(i, j int) bool {
		if slow[i].DurationMS == slow[j].DurationMS {
			return slow[i].ID < slow[j].ID
		}
		return slow[i].DurationMS > slow[j].DurationMS
	})
	if len(slow) > 5 {
		slow = slow[:5]
	}
	metrics.TopSlowVertices = slow

	patterns := make([]string, 0, len(missPatternCount))
	for pattern, count := range missPatternCount {
		if count > 1 {
			patterns = append(patterns, pattern)
		}
	}
	sort.Strings(patterns)
	metrics.RepeatedMissPatterns = patterns

	return metrics
}

func longestPath(topo []string, vertices map[string]backend.BuildVertex, adj map[string][]string) (int64, []string, int) {
	dpWeight := map[string]int64{}
	dpLen := map[string]int{}
	prev := map[string]string{}
	for _, id := range topo {
		dpWeight[id] = vertices[id].DurationMS
		dpLen[id] = 1
	}
	for _, id := range topo {
		for _, next := range adj[id] {
			candidateWeight := dpWeight[id] + vertices[next].DurationMS
			candidateLen := dpLen[id] + 1
			if candidateWeight > dpWeight[next] || (candidateWeight == dpWeight[next] && candidateLen > dpLen[next]) {
				dpWeight[next] = candidateWeight
				dpLen[next] = candidateLen
				prev[next] = id
			}
		}
	}

	bestID := ""
	var bestWeight int64
	bestLen := 0
	for _, id := range topo {
		if dpWeight[id] > bestWeight || (dpWeight[id] == bestWeight && dpLen[id] > bestLen) {
			bestWeight = dpWeight[id]
			bestLen = dpLen[id]
			bestID = id
		}
	}

	path := make([]string, 0, bestLen)
	cursor := bestID
	for cursor != "" {
		path = append(path, cursor)
		cursor = prev[cursor]
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return bestWeight, path, bestLen
}

func normalizePattern(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"0", "", "1", "", "2", "", "3", "", "4", "", "5", "", "6", "", "7", "", "8", "", "9", "",
	)
	name = replacer.Replace(name)
	name = strings.Join(strings.Fields(name), " ")
	if len(name) > 80 {
		name = name[:80]
	}
	return name
}
