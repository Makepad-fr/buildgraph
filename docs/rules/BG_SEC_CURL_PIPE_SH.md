# BG_SEC_CURL_PIPE_SH

- Dimension: `security`
- Severity: `critical`

## Summary

Remote scripts are piped directly into a shell.

## Why It Matters

This bypasses integrity verification and allows remote tampering to execute immediately.

## Typical Trigger

```dockerfile
RUN curl -fsSL https://example.com/install.sh | sh
```

## Recommended Fix

Download, verify checksum/signature, then execute.

```dockerfile
RUN curl -fsSLo /tmp/install.sh https://example.com/install.sh \
 && echo "<sha256>  /tmp/install.sh" | sha256sum -c - \
 && sh /tmp/install.sh
```

## Remediation Checklist

- Never pipe unverified remote scripts to shell.
- Verify integrity before execution.
- Prefer signed release assets when available.
