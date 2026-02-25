# Buildgraph Agent Notes

## Permanent Rules
- Build execution and detection must use Go libraries and APIs. Do not shell out to Docker/BuildKit CLIs for build operations.
- BuildKit is the primary backend for v0. Keep backend architecture pluggable for future providers like Buildah.
- Use Go stdlib `flag` for CLI parsing unless explicitly changed.
- Keep CLI output human-first by default with stable `--json` structured output.
- Telemetry must remain opt-in.
- Record repeated user preferences in this file and honor them in future implementation changes.

## Project Notes
- 2026-02-25: Repository bootstrapped from empty state for Buildgraph v0.
- 2026-02-25: Implemented backend registry abstraction with BuildKit backend as first provider.
- 2026-02-25: Added direct BuildKit and Docker-backed execution paths without shell command execution.
- 2026-02-25: Docker-backed implementation uses Moby Go client modules (`github.com/moby/moby/client` and API types), not shell command wrappers.
- 2026-02-25: Added analysis engine scope across performance, cacheability, reproducibility, security, and policy dimensions.
- 2026-02-25: Added auth/events/capabilities scaffolding for SaaS readiness with local-first defaults.
- 2026-02-25: Added SQLite run/findings/build/event persistence in local state DB.
- 2026-02-25: Added CI workflow with OS matrix unit tests and Linux BuildKit integration jobs.
- 2026-02-25: Direct BuildKit progress channel is owned/closed by BuildKit client; do not close `SolveStatus` channel manually in driver code.
- 2026-02-25: BuildKit local exporter must use `ExportEntry.OutputDir` (not `Attrs.dest`) to avoid \"output directory is required for local exporter\".
- 2026-02-25: CI policy: run workflows only on pushes to `main` and pull requests targeting `main`.
