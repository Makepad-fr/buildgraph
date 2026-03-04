# buildgraph

`buildgraph` is a BuildKit execution intelligence CLI: understand what your build is actually doing.

## Quick Start

### Prerequisites
- BuildKit endpoint reachable (`buildkitd` or Docker BuildKit).
- Go 1.25+ to build from source.
- Graphviz `dot` only if you want `report export --format svg`.

```bash
go build ./cmd/buildgraph
./buildgraph analyze run --context . --file Dockerfile --output local --local-dest ./out
./buildgraph report metrics --json
```

## Example Output

### Human output (`buildgraph report show`)

```text
Report generated: 2026-03-04T11:48:00Z
Command: analyze run
Backend: buildkit
Endpoint: unix:///run/buildkit/buildkitd.sock
Duration: 8234ms
Critical path: 5120ms
Cache hit ratio: 66.67%
Graph: complete (39 vertices, 41 edges)
Top slow vertices:
- [builder 6/8] RUN go test ./... (2140ms)
- [builder 7/8] RUN go build ./cmd/buildgraph (1660ms)
Findings: 2
```

### JSON output (`--json`)

```json
{
  "apiVersion": "buildgraph.dev/v2",
  "kind": "BuildReport",
  "metadata": {
    "command": "analyze run",
    "generatedAt": "2026-03-04T11:48:00Z"
  },
  "spec": {
    "context": ".",
    "file": "Dockerfile",
    "backend": "auto",
    "output": "local"
  },
  "status": {
    "phase": "completed",
    "summary": {
      "durationMs": 8234,
      "cacheHits": 24,
      "cacheMisses": 12
    },
    "result": {
      "command": "analyze run",
      "graphCompleteness": "complete"
    }
  }
}
```

## Command Surface

```bash
buildgraph analyze
buildgraph analyze run
buildgraph build

buildgraph report show --run-id <id> | --file <report.json>
buildgraph report metrics --run-id <id> | --file <report.json>
buildgraph report compare --base run:<id>|<file> --head run:<id>|<file>
buildgraph report trend --last 10
buildgraph report export --run-id <id> --format dot|svg --out <path>

buildgraph ci check --baseline-source git|ci-artifact|object-storage [...]
buildgraph ci github-action init [--write path]
buildgraph ci gitlab-ci init [--write path]

buildgraph backend list
buildgraph doctor
buildgraph auth login --user <user> --token <token>
buildgraph auth logout
buildgraph auth whoami
buildgraph config show
buildgraph version
```

## JSON Contract

All `--json` outputs use the v2 resource contract:
- `apiVersion: buildgraph.dev/v2`
- `kind`
- `metadata`
- `spec`
- `status`

Schemas are published under [`schema/v2`](./schema/v2):
- `resource.schema.json`
- `buildreport.schema.json`

## Configuration

Merge precedence:
1. flags
2. environment variables
3. project config (`.buildgraph.yaml`)
4. global config (`$XDG_CONFIG_HOME/buildgraph/config.yaml` or OS equivalent)
5. defaults

Sample:

```yaml
backend: auto
endpoint: ""
telemetry:
  enabled: false
  sink: noop
ci:
  baselineSource: git
  baselineFile: ./buildgraph-baseline.json
  thresholds:
    duration_total_pct: 10
    critical_path_pct: 10
    cache_hit_ratio_pp_drop: 10
    cache_miss_count_pct: 15
    warning_count_delta: 0
defaults:
  analyze:
    dockerfile: Dockerfile
    severityThreshold: low
    failOn: any
  build:
    dockerfile: Dockerfile
    output: image
    imageRef: ghcr.io/acme/app:dev
profiles:
  ci:
    backend: buildkit
    endpoint: unix:///run/buildkit/buildkitd.sock
```

## Development

```bash
go test ./...
```

## Notes
- Build execution and detection use Go APIs, not shell wrappers.
- Telemetry remains opt-in.
- BuildKit is the primary v0/v0.2 backend, with pluggable backend architecture for future providers.
