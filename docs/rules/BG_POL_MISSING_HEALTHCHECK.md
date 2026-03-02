# BG_POL_MISSING_HEALTHCHECK

- Dimension: `policy`
- Severity: `low`

## Summary

The Dockerfile has no `HEALTHCHECK`.

## Why It Matters

Schedulers cannot distinguish healthy from unhealthy containers as reliably.

## Typical Trigger

No `HEALTHCHECK` instruction in the final image stage.

## Recommended Fix

Define a lightweight health probe.

```dockerfile
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
  CMD curl -fsS http://127.0.0.1:8080/health || exit 1
```

## Remediation Checklist

- Use an application-level readiness endpoint.
- Keep probe fast and deterministic.
- Tune interval/timeout/retries for production behavior.
