# buildgraph

`buildgraph` is a Build Intelligence CLI for BuildKit-first workflows.

## 30-Second Quickstart

### Direct BuildKit socket

```bash
go build ./cmd/buildgraph

./buildgraph build \
  --context integration/fixtures \
  --file Dockerfile.integration \
  --output local \
  --local-dest /tmp/buildgraph-out \
  --endpoint unix:///run/buildkit/buildkitd.sock \
  --progress=json \
  --trace /tmp/buildgraph.trace.jsonl

./buildgraph graph --from /tmp/buildgraph.trace.jsonl --format dot --output /tmp/buildgraph.dot
./buildgraph top --from /tmp/buildgraph.trace.jsonl
```

### Docker Desktop / Docker Engine

```bash
go build ./cmd/buildgraph

./buildgraph build \
  --context integration/fixtures \
  --file Dockerfile.integration \
  --image-ref buildgraph/quickstart:dev \
  --progress=human \
  --trace ./buildgraph.trace.jsonl

./buildgraph graph --from ./buildgraph.trace.jsonl --format json
./buildgraph top --from ./buildgraph.trace.jsonl --limit 5
```

## Example Output (Sample)

```text
$ buildgraph top --from ./buildgraph.trace.jsonl
Vertices analyzed: 9

Slowest vertices:
1. 4823 ms  RUN apk add --no-cache build-base (sha256:...)
2. 2111 ms  RUN go build ./cmd/buildgraph (sha256:...)

Critical path: 7034 ms
1. FROM golang:1.26-alpine (112 ms)
2. RUN apk add --no-cache build-base (4823 ms)
3. RUN go build ./cmd/buildgraph (2111 ms)
```

## Commands

```bash
buildgraph analyze [--context .] [--file Dockerfile] [--severity-threshold low|medium|high|critical] [--fail-on policy|security|any] [--json]
buildgraph build [--context .] [--file Dockerfile] [--target NAME] [--platform linux/amd64] [--build-arg KEY=VALUE] [--secret id=foo,src=./foo.txt] [--output image|oci|local] [--image-ref REF] [--oci-dest PATH] [--local-dest PATH] [--backend auto|buildkit] [--endpoint URL] [--progress human|json|none] [--trace out.jsonl] [--json]
buildgraph graph --from out.jsonl [--format dot|svg|json] [--output PATH] [--json]
buildgraph top --from out.jsonl [--limit N] [--json]
buildgraph backend list
buildgraph doctor
buildgraph auth login --user <user> --token <token>
buildgraph auth logout
buildgraph auth whoami
buildgraph config show
buildgraph version
```

## JSON Output Contract (`--json`)

All machine-readable command output uses a versioned envelope:

```json
{
  "apiVersion": "buildgraph.dev/v1",
  "command": "build",
  "schemaVersion": "1",
  "timestamp": "2026-02-26T00:00:00Z",
  "durationMs": 1234,
  "result": {},
  "errors": []
}
```

## What Data Is Collected

`buildgraph` stores local state in a SQLite database to support diagnostics and history:
- run metadata (`command`, duration, exit code, success/failure)
- analysis findings
- build result metadata
- local events

`buildgraph` can also write local build traces (`--trace`) as JSONL.

## What Is Never Uploaded By Default

- no build context files are uploaded by default
- no findings/build metadata are uploaded by default
- no telemetry is sent unless explicitly enabled (`telemetry.enabled: true`)

Auth credentials are stored locally via OS keyring when available, with local file fallback.

## Install from Source

Requires Go 1.26+.

```bash
go build ./cmd/buildgraph
```

## Download Prebuilt Binaries

Artifacts are published automatically for every GitHub release.

### Linux (amd64)

```bash
curl -sSfL -o buildgraph_linux_amd64.tar.gz \
  https://github.com/Makepad-fr/buildgraph/releases/latest/download/buildgraph_linux_amd64.tar.gz
tar -xzf buildgraph_linux_amd64.tar.gz
sudo install -m 0755 buildgraph /usr/local/bin/buildgraph
```

### macOS (amd64)

```bash
curl -sSfL -o buildgraph_darwin_amd64.tar.gz \
  https://github.com/Makepad-fr/buildgraph/releases/latest/download/buildgraph_darwin_amd64.tar.gz
tar -xzf buildgraph_darwin_amd64.tar.gz
chmod +x buildgraph
mv buildgraph /usr/local/bin/buildgraph
```

### Windows (amd64)

```powershell
Invoke-WebRequest -Uri "https://github.com/Makepad-fr/buildgraph/releases/latest/download/buildgraph_windows_amd64.zip" -OutFile "buildgraph_windows_amd64.zip"
Expand-Archive -Path ".\\buildgraph_windows_amd64.zip" -DestinationPath ".\\buildgraph"
```

## Configuration

Default merge precedence:
1. flags
2. environment variables
3. project config (`.buildgraph.yaml`)
4. global config (`$XDG_CONFIG_HOME/buildgraph/config.yaml` or OS equivalent)
5. defaults

Sample config:

```yaml
backend: auto
endpoint: ""
telemetry:
  enabled: false
  sink: noop
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

- Build execution avoids shelling out to Docker/BuildKit CLIs.
- Docker-backed mode supports image export; direct BuildKit mode supports image/OCI/local exports.
