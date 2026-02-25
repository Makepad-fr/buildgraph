# buildgraph

`buildgraph` is a Build Intelligence CLI focused on BuildKit-first workflows.

## Goals
- BuildKit-first build orchestration and diagnostics.
- Dockerfile intelligence across performance, cacheability, reproducibility, security, and policy.
- Human-first output with stable `--json` mode.
- Extensible backend architecture for future providers such as Buildah.
- SaaS-ready foundations (auth/events/capabilities) with opt-in behavior.

## Install

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

## Commands

```bash
buildgraph analyze [--context .] [--file Dockerfile] [--severity-threshold low|medium|high|critical] [--fail-on policy|security|any] [--json]
buildgraph build [--context .] [--file Dockerfile] [--target NAME] [--platform linux/amd64] [--build-arg KEY=VALUE] [--secret id=foo,src=./foo.txt] [--output image|oci|local] [--image-ref REF] [--oci-dest PATH] [--local-dest PATH] [--backend auto|buildkit] [--endpoint URL] [--json]
buildgraph backend list
buildgraph doctor
buildgraph auth login --user <user> --token <token>
buildgraph auth logout
buildgraph auth whoami
buildgraph config show
buildgraph version
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
- Build execution avoids shelling out to external build commands.
- Docker-backed mode currently supports image export, while direct BuildKit mode supports image/OCI/local exports.
