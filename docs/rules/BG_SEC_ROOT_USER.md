# BG_SEC_ROOT_USER

- Dimension: `security`
- Severity: `high`

## Summary

The resulting image runs as root.

## Why It Matters

Running as root increases impact of runtime compromise.

## Typical Trigger

No `USER` instruction, or `USER root` in final stage.

## Recommended Fix

Create a dedicated runtime user and switch to it in the final stage.

```dockerfile
RUN addgroup --system app && adduser --system --ingroup app app
USER app
```

## Remediation Checklist

- Set `USER` in the final runtime stage.
- Ensure runtime paths are writable by that user.
- Keep root-only operations in build stages.
