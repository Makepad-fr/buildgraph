package trace

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var ErrNoVertexData = errors.New("trace does not contain vertex IDs; direct BuildKit progress is required for graph analysis")

type Vertex struct {
	ID         string     `json:"id"`
	Name       string     `json:"name,omitempty"`
	Inputs     []string   `json:"inputs,omitempty"`
	Started    *time.Time `json:"started,omitempty"`
	Completed  *time.Time `json:"completed,omitempty"`
	Cached     bool       `json:"cached"`
	Error      string     `json:"error,omitempty"`
	DurationMS int64      `json:"durationMs"`
}

type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Graph struct {
	Vertices []Vertex `json:"vertices"`
	Edges    []Edge   `json:"edges"`
}

type VertexSummary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	DurationMS int64  `json:"durationMs"`
	Cached     bool   `json:"cached"`
	Error      string `json:"error,omitempty"`
}

type CriticalPath struct {
	DurationMS int64           `json:"durationMs"`
	Vertices   []VertexSummary `json:"vertices"`
}

type TopResult struct {
	VertexCount   int             `json:"vertexCount"`
	Slowest       []VertexSummary `json:"slowest"`
	CriticalPath  CriticalPath    `json:"criticalPath"`
	HasCycle      bool            `json:"hasCycle"`
	CycleDetected string          `json:"cycleDetected,omitempty"`
}

func BuildGraph(records []Record) (Graph, error) {
	vertices := map[string]*Vertex{}

	getVertex := func(id string) *Vertex {
		if existing, ok := vertices[id]; ok {
			return existing
		}
		created := &Vertex{ID: id}
		vertices[id] = created
		return created
	}

	for _, rec := range records {
		if rec.Kind != KindProgress || rec.Progress == nil {
			continue
		}
		progress := rec.Progress
		vertexID := strings.TrimSpace(progress.VertexID)
		if vertexID == "" {
			continue
		}

		vertex := getVertex(vertexID)
		if message := strings.TrimSpace(progress.Message); message != "" {
			vertex.Name = message
		}
		if len(progress.Inputs) > 0 {
			for _, input := range progress.Inputs {
				trimmed := strings.TrimSpace(input)
				if trimmed == "" || trimmed == vertexID {
					continue
				}
				if !contains(vertex.Inputs, trimmed) {
					vertex.Inputs = append(vertex.Inputs, trimmed)
				}
			}
		}
		if progress.Started != nil && (vertex.Started == nil || progress.Started.Before(*vertex.Started)) {
			vertex.Started = cloneTime(progress.Started)
		}
		if progress.Completed != nil && (vertex.Completed == nil || progress.Completed.After(*vertex.Completed)) {
			vertex.Completed = cloneTime(progress.Completed)
		}
		if progress.Cached {
			vertex.Cached = true
		}
		if errText := strings.TrimSpace(progress.Error); errText != "" {
			vertex.Error = errText
		}
	}

	if len(vertices) == 0 {
		return Graph{}, ErrNoVertexData
	}

	edgeKeys := map[string]struct{}{}
	edges := make([]Edge, 0)
	for _, vertex := range vertices {
		sort.Strings(vertex.Inputs)
		for _, input := range vertex.Inputs {
			_ = getVertex(input)
			key := input + "->" + vertex.ID
			if _, ok := edgeKeys[key]; ok {
				continue
			}
			edgeKeys[key] = struct{}{}
			edges = append(edges, Edge{From: input, To: vertex.ID})
		}
		if vertex.Started != nil && vertex.Completed != nil && vertex.Completed.After(*vertex.Started) {
			vertex.DurationMS = vertex.Completed.Sub(*vertex.Started).Milliseconds()
		}
	}

	vertexIDs := make([]string, 0, len(vertices))
	for id := range vertices {
		vertexIDs = append(vertexIDs, id)
	}
	sort.Strings(vertexIDs)
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From == edges[j].From {
			return edges[i].To < edges[j].To
		}
		return edges[i].From < edges[j].From
	})

	result := Graph{
		Vertices: make([]Vertex, 0, len(vertexIDs)),
		Edges:    edges,
	}
	for _, id := range vertexIDs {
		result.Vertices = append(result.Vertices, *vertices[id])
	}
	return result, nil
}

func AnalyzeTop(graph Graph, limit int) TopResult {
	if limit <= 0 {
		limit = 10
	}

	verticesByID := make(map[string]Vertex, len(graph.Vertices))
	for _, vertex := range graph.Vertices {
		verticesByID[vertex.ID] = vertex
	}

	summaries := make([]VertexSummary, 0, len(graph.Vertices))
	for _, vertex := range graph.Vertices {
		summaries = append(summaries, summarizeVertex(vertex))
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].DurationMS == summaries[j].DurationMS {
			if summaries[i].Name == summaries[j].Name {
				return summaries[i].ID < summaries[j].ID
			}
			return summaries[i].Name < summaries[j].Name
		}
		return summaries[i].DurationMS > summaries[j].DurationMS
	})
	if limit > len(summaries) {
		limit = len(summaries)
	}

	pathIDs, durationMS, hasCycle := longestPath(graph)
	pathSummaries := make([]VertexSummary, 0, len(pathIDs))
	for _, id := range pathIDs {
		vertex, ok := verticesByID[id]
		if !ok {
			continue
		}
		pathSummaries = append(pathSummaries, summarizeVertex(vertex))
	}

	result := TopResult{
		VertexCount:  len(graph.Vertices),
		Slowest:      summaries[:limit],
		CriticalPath: CriticalPath{DurationMS: durationMS, Vertices: pathSummaries},
		HasCycle:     hasCycle,
	}
	if hasCycle {
		result.CycleDetected = "trace graph contains cycles; critical path used fallback ordering"
	}
	return result
}

func DOT(graph Graph) string {
	var builder strings.Builder
	builder.WriteString("digraph buildgraph {\n")

	for _, vertex := range graph.Vertices {
		label := vertex.Name
		if label == "" {
			label = vertex.ID
		}
		if vertex.DurationMS > 0 {
			label = fmt.Sprintf("%s\\n%d ms", label, vertex.DurationMS)
		}
		attrs := []string{fmt.Sprintf("label=\"%s\"", escapeDOT(label))}
		if vertex.Cached {
			attrs = append(attrs, "style=\"dashed\"")
		}
		if vertex.Error != "" {
			attrs = append(attrs, "color=\"red\"")
		}
		builder.WriteString(fmt.Sprintf("  \"%s\" [%s];\n", escapeDOT(vertex.ID), strings.Join(attrs, ",")))
	}

	for _, edge := range graph.Edges {
		builder.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\";\n", escapeDOT(edge.From), escapeDOT(edge.To)))
	}
	builder.WriteString("}\n")
	return builder.String()
}

func longestPath(graph Graph) ([]string, int64, bool) {
	ids := make([]string, 0, len(graph.Vertices))
	durationByID := map[string]int64{}
	inDegree := map[string]int{}
	adjacent := map[string][]string{}
	for _, vertex := range graph.Vertices {
		ids = append(ids, vertex.ID)
		durationByID[vertex.ID] = vertex.DurationMS
		inDegree[vertex.ID] = 0
	}
	sort.Strings(ids)

	for _, edge := range graph.Edges {
		adjacent[edge.From] = append(adjacent[edge.From], edge.To)
		inDegree[edge.To]++
		if _, ok := inDegree[edge.From]; !ok {
			inDegree[edge.From] = 0
			ids = append(ids, edge.From)
			durationByID[edge.From] = 0
		}
		if _, ok := inDegree[edge.To]; !ok {
			inDegree[edge.To] = 0
			ids = append(ids, edge.To)
			durationByID[edge.To] = 0
		}
	}
	sort.Strings(ids)

	queue := make([]string, 0)
	for _, id := range ids {
		if inDegree[id] == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue)

	topo := make([]string, 0, len(ids))
	for len(queue) > 0 {
		sort.Strings(queue)
		current := queue[0]
		queue = queue[1:]
		topo = append(topo, current)
		for _, next := range adjacent[current] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	hasCycle := len(topo) != len(ids)
	if hasCycle {
		bestID := ""
		var bestDuration int64
		for _, id := range ids {
			if durationByID[id] > bestDuration || bestID == "" {
				bestID = id
				bestDuration = durationByID[id]
			}
		}
		if bestID == "" {
			return nil, 0, true
		}
		return []string{bestID}, bestDuration, true
	}

	distance := map[string]int64{}
	previous := map[string]string{}
	for _, id := range topo {
		distance[id] = durationByID[id]
	}
	for _, current := range topo {
		for _, next := range adjacent[current] {
			candidate := distance[current] + durationByID[next]
			if candidate > distance[next] {
				distance[next] = candidate
				previous[next] = current
			}
		}
	}

	end := ""
	var maxDistance int64
	for _, id := range topo {
		if distance[id] > maxDistance || end == "" {
			end = id
			maxDistance = distance[id]
		}
	}
	if end == "" {
		return nil, 0, false
	}

	path := make([]string, 0)
	for current := end; current != ""; current = previous[current] {
		path = append(path, current)
		if _, ok := previous[current]; !ok {
			break
		}
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path, maxDistance, false
}

func summarizeVertex(vertex Vertex) VertexSummary {
	name := vertex.Name
	if name == "" {
		name = vertex.ID
	}
	return VertexSummary{
		ID:         vertex.ID,
		Name:       name,
		DurationMS: vertex.DurationMS,
		Cached:     vertex.Cached,
		Error:      vertex.Error,
	}
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := value.UTC()
	return &copied
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func escapeDOT(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return replacer.Replace(value)
}
