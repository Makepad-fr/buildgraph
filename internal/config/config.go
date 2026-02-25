package config

import "time"

type Config struct {
	Backend   string                   `yaml:"backend" json:"backend"`
	Endpoint  string                   `yaml:"endpoint" json:"endpoint"`
	Telemetry TelemetryConfig          `yaml:"telemetry" json:"telemetry"`
	Auth      AuthConfig               `yaml:"auth" json:"auth"`
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
		Profiles: map[string]ProfileConfig{},
	}
}
