# BG_PERF_APT_SPLIT

- Dimension: `performance`
- Severity: `medium`

## Summary

`apt-get update` is executed without an install in the same layer.

## Why It Matters

Splitting update and install into separate layers increases rebuild time and can use stale package indexes.

## Typical Trigger

```dockerfile
RUN apt-get update
RUN apt-get install -y curl
```

## Recommended Fix

Combine update and install in one instruction and clear apt lists:

```dockerfile
RUN apt-get update \
 && apt-get install -y --no-install-recommends curl \
 && rm -rf /var/lib/apt/lists/*
```

## Remediation Checklist

- Merge `apt-get update` with related install commands.
- Add `--no-install-recommends` where possible.
- Remove apt cache in the same layer.
