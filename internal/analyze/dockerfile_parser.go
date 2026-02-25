package analyze

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/parser"
)

type Instruction struct {
	Command string
	Value   string
	Raw     string
	Line    int
}

type ParsedDockerfile struct {
	Path         string
	Instructions []Instruction
	Lines        []string
}

func ParseDockerfile(contextDir, dockerfilePath string) (ParsedDockerfile, error) {
	resolvedPath := dockerfilePath
	if !filepath.IsAbs(resolvedPath) {
		resolvedPath = filepath.Join(contextDir, dockerfilePath)
	}

	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return ParsedDockerfile{}, fmt.Errorf("read dockerfile: %w", err)
	}

	result, err := parser.Parse(bytes.NewReader(content))
	if err != nil {
		return ParsedDockerfile{}, fmt.Errorf("parse dockerfile: %w", err)
	}

	instructions := make([]Instruction, 0, len(result.AST.Children))
	for _, child := range result.AST.Children {
		value := ""
		if child.Next != nil {
			value = strings.TrimSpace(child.Next.Value)
		}
		instructions = append(instructions, Instruction{
			Command: strings.ToUpper(strings.TrimSpace(child.Value)),
			Value:   value,
			Raw:     strings.TrimSpace(child.Original),
			Line:    child.StartLine,
		})
	}

	return ParsedDockerfile{
		Path:         resolvedPath,
		Instructions: instructions,
		Lines:        strings.Split(string(content), "\n"),
	}, nil
}
