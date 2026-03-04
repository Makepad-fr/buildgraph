package report

import (
	"encoding/json"
	"fmt"
	"os"
)

type resourceStatus struct {
	Result json.RawMessage `json:"result"`
}

type resourceEnvelope struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Status     resourceStatus `json:"status"`
}

func ReadBuildReportFile(path string) (BuildReport, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return BuildReport{}, fmt.Errorf("read report: %w", err)
	}
	return ParseBuildReportJSON(payload)
}

func ParseBuildReportJSON(payload []byte) (BuildReport, error) {
	var direct BuildReport
	if err := json.Unmarshal(payload, &direct); err == nil {
		if direct.Command != "" || direct.Backend != "" || direct.GeneratedAt.Unix() != 0 {
			return direct, nil
		}
	}

	var envelope resourceEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return BuildReport{}, fmt.Errorf("parse report: %w", err)
	}
	if len(envelope.Status.Result) == 0 {
		return BuildReport{}, fmt.Errorf("report JSON does not contain status.result")
	}
	if err := json.Unmarshal(envelope.Status.Result, &direct); err != nil {
		return BuildReport{}, fmt.Errorf("parse report payload: %w", err)
	}
	return direct, nil
}
