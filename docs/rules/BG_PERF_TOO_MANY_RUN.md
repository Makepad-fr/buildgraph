# BG_PERF_TOO_MANY_RUN

- Dimension: `performance`
- Severity: `low`

## Summary

The Dockerfile has many `RUN` instructions, increasing layer count.

## Why It Matters

Too many layers can slow builds and make images harder to reason about.

## Typical Trigger

Many small `RUN` instructions that could be grouped.

## Recommended Fix

Consolidate related commands while keeping readability and cache boundaries sensible.

```dockerfile
RUN apk add --no-cache bash curl git \
 && update-ca-certificates
```

## Remediation Checklist

- Merge only logically related steps.
- Keep package install and cleanup in the same layer.
- Avoid over-consolidating unrelated steps.
