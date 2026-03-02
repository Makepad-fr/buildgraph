# BG_REPRO_FROM_MUTABLE

- Dimension: `reproducibility`
- Severity: `high`

## Summary

A `FROM` image uses a mutable tag and is not pinned to a digest.

## Why It Matters

Mutable tags can drift over time, causing non-deterministic builds.

## Typical Trigger

```dockerfile
FROM alpine:3.20
```

## Recommended Fix

Pin images to immutable digests.

```dockerfile
FROM alpine:3.20@sha256:<digest>
```

## Remediation Checklist

- Pin all stage base images.
- Automate digest refresh using a scheduled dependency workflow.
- Keep tag plus digest for readability.
