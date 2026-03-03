# Buildgraph

Buildgraph is a Build Intelligence CLI for BuildKit-first workflows.

## Install

### From source (Go 1.26+)

```bash
go build ./cmd/buildgraph
```

### Prebuilt binaries

#### Linux (amd64)

```bash
curl -sSfL -o buildgraph_linux_amd64.tar.gz \
  https://github.com/Makepad-fr/buildgraph/releases/latest/download/buildgraph_linux_amd64.tar.gz
tar -xzf buildgraph_linux_amd64.tar.gz
sudo install -m 0755 buildgraph /usr/local/bin/buildgraph
```

#### macOS (amd64)

```bash
curl -sSfL -o buildgraph_darwin_amd64.tar.gz \
  https://github.com/Makepad-fr/buildgraph/releases/latest/download/buildgraph_darwin_amd64.tar.gz
tar -xzf buildgraph_darwin_amd64.tar.gz
chmod +x buildgraph
mv buildgraph /usr/local/bin/buildgraph
```

#### Windows (amd64)

```powershell
Invoke-WebRequest -Uri "https://github.com/Makepad-fr/buildgraph/releases/latest/download/buildgraph_windows_amd64.zip" -OutFile "buildgraph_windows_amd64.zip"
Expand-Archive -Path ".\\buildgraph_windows_amd64.zip" -DestinationPath ".\\buildgraph"
```

## 30-Second Quickstart

```bash
buildgraph build \
  --context . \
  --file Dockerfile \
  --image-ref ghcr.io/acme/app:dev \
  --progress=human \
  --trace ./buildgraph.trace.jsonl

buildgraph top --from ./buildgraph.trace.jsonl --limit 5
```

## Documentation

- [Rules Overview](./rules/index.md)

Rule links emitted by `buildgraph analyze` resolve under:

- `https://buildgraph.dev/rules/<RULE_ID>`
