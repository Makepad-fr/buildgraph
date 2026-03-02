# BG_REPRO_APT_UNPINNED

- Dimension: `reproducibility`
- Severity: `medium`

## Summary

`apt-get install` installs packages without explicit versions.

## Why It Matters

Package versions can change across mirror updates, producing different images over time.

## Typical Trigger

```dockerfile
RUN apt-get update && apt-get install -y curl ca-certificates
```

## Recommended Fix

Pin versions where practical, or use a controlled artifact/mirror policy.

```dockerfile
RUN apt-get update \
 && apt-get install -y curl=8.5.0-2ubuntu10 ca-certificates=20240203 \
 && rm -rf /var/lib/apt/lists/*
```

## Remediation Checklist

- Pin critical packages.
- Align with your distro release cadence.
- Use internal mirrors for strict reproducibility.
