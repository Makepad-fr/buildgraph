package analyze

import (
	"strconv"
	"strings"
)

type StageNode struct {
	Name string `json:"name"`
	Base string `json:"base,omitempty"`
	Line int    `json:"line"`
}

type StageEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason"`
}

type StageGraph struct {
	Stages []StageNode `json:"stages"`
	Edges  []StageEdge `json:"edges"`
}

func ParseStageGraph(contextDir, dockerfilePath string) (StageGraph, error) {
	parsed, err := ParseDockerfile(contextDir, dockerfilePath)
	if err != nil {
		return StageGraph{}, err
	}

	graph := StageGraph{
		Stages: make([]StageNode, 0, 8),
		Edges:  make([]StageEdge, 0, 16),
	}
	stageExists := map[string]bool{}
	edgeSet := map[string]bool{}
	currentStage := ""

	for index, inst := range parsed.Instructions {
		switch inst.Command {
		case "FROM":
			base, alias := parseFromInstruction(instructionBody(inst.Raw, "FROM", inst.Value))
			stageName := alias
			if stageName == "" {
				stageName = "stage-" + strconv.Itoa(index)
			}
			if !stageExists[stageName] {
				stageExists[stageName] = true
				graph.Stages = append(graph.Stages, StageNode{
					Name: stageName,
					Base: base,
					Line: inst.Line,
				})
			}
			if base != "" && stageExists[base] {
				graph.Edges = addStageEdge(graph.Edges, edgeSet, StageEdge{From: base, To: stageName, Reason: "from"})
			}
			currentStage = stageName
		case "COPY", "ADD":
			source := parseCopyFrom(instructionBody(inst.Raw, inst.Command, inst.Value))
			if source != "" && currentStage != "" && stageExists[source] {
				graph.Edges = addStageEdge(graph.Edges, edgeSet, StageEdge{From: source, To: currentStage, Reason: "copy-from"})
			}
		}
	}

	return graph, nil
}

func instructionBody(raw, command, fallback string) string {
	trimmedRaw := strings.TrimSpace(raw)
	if trimmedRaw == "" {
		return strings.TrimSpace(fallback)
	}
	command = strings.ToUpper(strings.TrimSpace(command))
	prefix := command + " "
	upperRaw := strings.ToUpper(trimmedRaw)
	if strings.HasPrefix(upperRaw, prefix) {
		return strings.TrimSpace(trimmedRaw[len(prefix):])
	}
	return strings.TrimSpace(fallback)
}

func parseFromInstruction(value string) (base string, alias string) {
	parts := strings.Fields(strings.TrimSpace(value))
	if len(parts) == 0 {
		return "", ""
	}
	base = parts[0]
	if len(parts) >= 3 && strings.EqualFold(parts[1], "as") {
		alias = parts[2]
	}
	return base, alias
}

func parseCopyFrom(value string) string {
	for _, part := range strings.Fields(value) {
		if !strings.HasPrefix(strings.ToLower(part), "--from=") {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(part, "--from="))
	}
	return ""
}

func addStageEdge(edges []StageEdge, edgeSet map[string]bool, edge StageEdge) []StageEdge {
	if edge.From == "" || edge.To == "" || edge.From == edge.To {
		return edges
	}
	key := edge.From + "->" + edge.To + ":" + edge.Reason
	if edgeSet[key] {
		return edges
	}
	edgeSet[key] = true
	return append(edges, edge)
}
