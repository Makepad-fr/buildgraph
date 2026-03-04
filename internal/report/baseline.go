package report

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type BaselineOptions struct {
	Source string
	File   string
	URL    string
}

func LoadBaseline(opts BaselineOptions) (BuildReport, error) {
	source := strings.ToLower(strings.TrimSpace(opts.Source))
	switch source {
	case "git", "ci-artifact":
		if strings.TrimSpace(opts.File) == "" {
			return BuildReport{}, fmt.Errorf("--baseline-file is required for baseline-source=%s", source)
		}
		return ReadBuildReportFile(opts.File)
	case "object-storage":
		if strings.TrimSpace(opts.URL) == "" {
			return BuildReport{}, fmt.Errorf("--baseline-url is required for baseline-source=object-storage")
		}
		return readBuildReportURL(opts.URL)
	default:
		return BuildReport{}, fmt.Errorf("unsupported baseline source %q", opts.Source)
	}
}

func readBuildReportURL(address string) (BuildReport, error) {
	if strings.HasPrefix(address, "file://") {
		path := strings.TrimPrefix(address, "file://")
		return ReadBuildReportFile(path)
	}
	if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
		if _, err := os.Stat(address); err == nil {
			return ReadBuildReportFile(address)
		}
		return BuildReport{}, fmt.Errorf("baseline url must be http(s), file://, or existing local file")
	}
	resp, err := http.Get(address)
	if err != nil {
		return BuildReport{}, fmt.Errorf("download baseline report: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return BuildReport{}, fmt.Errorf("download baseline report: %s", resp.Status)
	}
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return BuildReport{}, fmt.Errorf("read baseline response: %w", err)
	}
	return ParseBuildReportJSON(payload)
}
