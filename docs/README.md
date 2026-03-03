# Buildgraph Documentation Source

This directory contains the source content for:

- the landing page (`docs/index.md`)
- rule pages used by `buildgraph analyze` finding links

## Rules

- [Rules Index](./rules/index.md)

Each rule page is named after its rule ID, for example:
- `docs/rules/BG_REPRO_FROM_MUTABLE.md`
- `docs/rules/BG_SEC_ROOT_USER.md`

Rule pages are designed to map directly to public URLs under:
- `https://buildgraph.dev/rules/<RULE_ID>`

## Domain Note

The current Pages custom domain file is:

- `docs/CNAME` (currently set to `buildgraph.dev`)

If you want to move docs to a subdomain later, update this file and DNS records accordingly.
