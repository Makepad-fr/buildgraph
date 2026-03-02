# BG_CACHE_ARG_LATE

- Dimension: `cacheability`
- Severity: `medium`

## Summary

`ARG` values are declared late, after expensive build steps.

## Why It Matters

Changing late `ARG` values can invalidate large portions of the build graph.

## Typical Trigger

```dockerfile
RUN make deps
ARG APP_VERSION
RUN make build
```

## Recommended Fix

Declare stable `ARG` values before expensive operations.

```dockerfile
ARG APP_VERSION
RUN make deps
RUN make build
```

## Remediation Checklist

- Move stable args earlier.
- Keep volatile args scoped only where needed.
- Re-check cache hit rate in CI.
