# BG_CACHE_COPY_ALL_EARLY

- Dimension: `cacheability`
- Severity: `high`

## Summary

A broad `COPY .` happens before dependency install steps.

## Why It Matters

Any source file change invalidates dependency cache and forces expensive reinstall/build steps.

## Typical Trigger

```dockerfile
COPY . .
RUN npm ci
```

## Recommended Fix

Copy dependency manifests first, install dependencies, then copy the remaining source.

```dockerfile
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
```

## Remediation Checklist

- Move lockfiles/manifests before install steps.
- Keep app source copy after dependency restore.
- Add `.dockerignore` to reduce noisy context changes.
