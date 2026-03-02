# BG_POL_MISSING_SOURCE_LABEL

- Dimension: `policy`
- Severity: `medium`

## Summary

The image does not include `org.opencontainers.image.source`.

## Why It Matters

Missing provenance metadata reduces traceability and compliance reporting quality.

## Typical Trigger

No OCI source label is present.

## Recommended Fix

Set the source repository label in the final image stage.

```dockerfile
LABEL org.opencontainers.image.source="https://github.com/acme/service"
```

## Remediation Checklist

- Add OCI source label to final stage.
- Keep label aligned with canonical repository URL.
- Add additional OCI labels where useful (revision, created, version).
