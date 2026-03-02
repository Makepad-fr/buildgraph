# BG_SEC_PLAIN_SECRET_ENV

- Dimension: `security`
- Severity: `critical`

## Summary

A likely secret is stored with `ENV`, embedding it into image metadata/history.

## Why It Matters

Secrets become recoverable from image layers, registry metadata, and build logs.

## Typical Trigger

```dockerfile
ENV AWS_SECRET_ACCESS_KEY=...
```

## Recommended Fix

Use BuildKit secret mounts at build time, and runtime secret injection for containers.

```dockerfile
# build: --secret id=npm_token,src=.npm_token
RUN --mount=type=secret,id=npm_token \
    NPM_TOKEN="$(cat /run/secrets/npm_token)" npm ci
```

## Remediation Checklist

- Remove secrets from `ENV` and `ARG` where possible.
- Use BuildKit `--secret` for build-time credentials.
- Rotate any leaked credentials.
