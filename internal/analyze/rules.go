package analyze

import (
	"github.com/Makepad-fr/buildgraph/internal/backend"
	"github.com/Makepad-fr/buildgraph/internal/policy"
)

const docsBase = "https://buildgraph.dev/rules/"

func BuiltinRules() map[string]policy.Rule {
	return map[string]policy.Rule{
		"BG_PERF_APT_SPLIT": {
			ID:         "BG_PERF_APT_SPLIT",
			Dimension:  backend.DimensionPerformance,
			Severity:   backend.SeverityMedium,
			Matcher:    "RUN apt-get update without install",
			Message:    "Combine apt-get update with apt-get install in a single RUN to avoid stale package indexes.",
			Suggestion: "Use one RUN: apt-get update && apt-get install -y <packages> && rm -rf /var/lib/apt/lists/*",
			DocsRef:    docsBase + "BG_PERF_APT_SPLIT",
		},
		"BG_PERF_TOO_MANY_RUN": {
			ID:         "BG_PERF_TOO_MANY_RUN",
			Dimension:  backend.DimensionPerformance,
			Severity:   backend.SeverityLow,
			Matcher:    "More than five RUN instructions",
			Message:    "Many RUN layers increase build overhead and image complexity.",
			Suggestion: "Consolidate related commands where practical.",
			DocsRef:    docsBase + "BG_PERF_TOO_MANY_RUN",
		},
		"BG_CACHE_COPY_ALL_EARLY": {
			ID:         "BG_CACHE_COPY_ALL_EARLY",
			Dimension:  backend.DimensionCacheability,
			Severity:   backend.SeverityHigh,
			Matcher:    "COPY . before dependency install",
			Message:    "Copying the full context before installing dependencies often invalidates cache on every source change.",
			Suggestion: "Copy dependency manifests first, install dependencies, then copy the remaining source.",
			DocsRef:    docsBase + "BG_CACHE_COPY_ALL_EARLY",
		},
		"BG_CACHE_ARG_LATE": {
			ID:         "BG_CACHE_ARG_LATE",
			Dimension:  backend.DimensionCacheability,
			Severity:   backend.SeverityMedium,
			Matcher:    "ARG introduced after build-critical RUN",
			Message:    "Late ARG declarations can reduce cache reuse across builds.",
			Suggestion: "Declare stable ARG values earlier, before expensive RUN steps.",
			DocsRef:    docsBase + "BG_CACHE_ARG_LATE",
		},
		"BG_REPRO_FROM_MUTABLE": {
			ID:         "BG_REPRO_FROM_MUTABLE",
			Dimension:  backend.DimensionReproducibility,
			Severity:   backend.SeverityHigh,
			Matcher:    "FROM image without immutable reference",
			Message:    "Base image is not pinned to a digest or fixed version tag.",
			Suggestion: "Pin image to an immutable digest, for example alpine@sha256:<digest>.",
			DocsRef:    docsBase + "BG_REPRO_FROM_MUTABLE",
		},
		"BG_REPRO_APT_UNPINNED": {
			ID:         "BG_REPRO_APT_UNPINNED",
			Dimension:  backend.DimensionReproducibility,
			Severity:   backend.SeverityMedium,
			Matcher:    "apt-get install unpinned packages",
			Message:    "Unpinned package installs can produce non-deterministic outputs.",
			Suggestion: "Pin package versions or use locked artifact repositories.",
			DocsRef:    docsBase + "BG_REPRO_APT_UNPINNED",
		},
		"BG_SEC_ROOT_USER": {
			ID:         "BG_SEC_ROOT_USER",
			Dimension:  backend.DimensionSecurity,
			Severity:   backend.SeverityHigh,
			Matcher:    "Container runs as root",
			Message:    "Image runs as root user.",
			Suggestion: "Create and switch to a non-root USER for runtime.",
			DocsRef:    docsBase + "BG_SEC_ROOT_USER",
		},
		"BG_SEC_CURL_PIPE_SH": {
			ID:         "BG_SEC_CURL_PIPE_SH",
			Dimension:  backend.DimensionSecurity,
			Severity:   backend.SeverityCritical,
			Matcher:    "curl/wget piped to shell",
			Message:    "Piping remote scripts directly into shell bypasses integrity checks.",
			Suggestion: "Download artifact, verify checksum/signature, then execute.",
			DocsRef:    docsBase + "BG_SEC_CURL_PIPE_SH",
		},
		"BG_SEC_PLAIN_SECRET_ENV": {
			ID:         "BG_SEC_PLAIN_SECRET_ENV",
			Dimension:  backend.DimensionSecurity,
			Severity:   backend.SeverityCritical,
			Matcher:    "ENV contains likely secret",
			Message:    "Potential secret-like value is baked into image metadata.",
			Suggestion: "Use BuildKit secrets mounts or runtime secret injection.",
			DocsRef:    docsBase + "BG_SEC_PLAIN_SECRET_ENV",
		},
		"BG_POL_MISSING_SOURCE_LABEL": {
			ID:         "BG_POL_MISSING_SOURCE_LABEL",
			Dimension:  backend.DimensionPolicy,
			Severity:   backend.SeverityMedium,
			Matcher:    "Missing OCI source label",
			Message:    "Image is missing org.opencontainers.image.source label.",
			Suggestion: "Add LABEL org.opencontainers.image.source=<repository-url>.",
			DocsRef:    docsBase + "BG_POL_MISSING_SOURCE_LABEL",
		},
		"BG_POL_MISSING_HEALTHCHECK": {
			ID:         "BG_POL_MISSING_HEALTHCHECK",
			Dimension:  backend.DimensionPolicy,
			Severity:   backend.SeverityLow,
			Matcher:    "Missing HEALTHCHECK",
			Message:    "Container has no HEALTHCHECK instruction.",
			Suggestion: "Define a HEALTHCHECK for operational safety.",
			DocsRef:    docsBase + "BG_POL_MISSING_HEALTHCHECK",
		},
	}
}
