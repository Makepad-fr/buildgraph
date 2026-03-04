package config

import "time"

type Config struct {
	Backend   string                   `yaml:"backend" json:"backend"`
	Endpoint  string                   `yaml:"endpoint" json:"endpoint"`
	Telemetry TelemetryConfig          `yaml:"telemetry" json:"telemetry"`
	Auth      AuthConfig               `yaml:"auth" json:"auth"`
	CI        CIConfig                 `yaml:"ci" json:"ci"`
	Defaults  DefaultsConfig           `yaml:"defaults" json:"defaults"`
	Profiles  map[string]ProfileConfig `yaml:"profiles" json:"profiles"`
}

type TelemetryConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Sink    string `yaml:"sink" json:"sink"`
}

type AuthConfig struct {
	Endpoint string `yaml:"endpoint" json:"endpoint"`
	User     string `yaml:"user" json:"user"`
}

type CIConfig struct {
	BaselineSource string             `yaml:"baselineSource" json:"baselineSource"`
	BaselineFile   string             `yaml:"baselineFile" json:"baselineFile"`
	BaselineURL    string             `yaml:"baselineUrl" json:"baselineUrl"`
	Thresholds     map[string]float64 `yaml:"thresholds" json:"thresholds"`
}

type DefaultsConfig struct {
	Analyze AnalyzeDefaults `yaml:"analyze" json:"analyze"`
	Build   BuildDefaults   `yaml:"build" json:"build"`
}

type AnalyzeDefaults struct {
	Dockerfile        string `yaml:"dockerfile" json:"dockerfile"`
	SeverityThreshold string `yaml:"severityThreshold" json:"severityThreshold"`
	FailOn            string `yaml:"failOn" json:"failOn"`
}

type BuildDefaults struct {
	Dockerfile string `yaml:"dockerfile" json:"dockerfile"`
	OutputMode string `yaml:"output" json:"output"`
	ImageRef   string `yaml:"imageRef" json:"imageRef"`
}

type ProfileConfig struct {
	Backend  string `yaml:"backend" json:"backend"`
	Endpoint string `yaml:"endpoint" json:"endpoint"`
}

type Override struct {
	Backend  string
	Endpoint string
}

type Paths struct {
	GlobalPath    string `json:"globalPath"`
	ProjectPath   string `json:"projectPath"`
	GlobalExists  bool   `json:"globalExists"`
	ProjectExists bool   `json:"projectExists"`
}

type Loaded struct {
	Config   Config    `json:"config"`
	Paths    Paths     `json:"paths"`
	LoadedAt time.Time `json:"loadedAt"`
}

func DefaultConfig() Config {
	return Config{
		Backend:  "auto",
		Endpoint: "",
		Telemetry: TelemetryConfig{
			Enabled: false,
			Sink:    "noop",
		},
		Defaults: DefaultsConfig{
			Analyze: AnalyzeDefaults{
				Dockerfile:        "Dockerfile",
				SeverityThreshold: "low",
				FailOn:            "any",
			},
			Build: BuildDefaults{
				Dockerfile: "Dockerfile",
				OutputMode: "image",
				ImageRef:   "",
			},
		},
		CI: CIConfig{
			Thresholds: map[string]float64{
				"duration_total_pct":      10,
				"critical_path_pct":       10,
				"cache_hit_ratio_pp_drop": 10,
				"cache_miss_count_pct":    15,
				"warning_count_delta":     0,
			},
		},
		Profiles: map[string]ProfileConfig{},
	}
}
