package analyze

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestBuiltinRulesHaveDocsPages(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve current file for docs path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	docsDir := filepath.Join(repoRoot, "docs", "rules")

	rules := BuiltinRules()
	missing := make([]string, 0)
	for id, rule := range rules {
		expectedURL := docsBase + id
		if rule.DocsRef != expectedURL {
			t.Fatalf("unexpected docs url for %s: got=%q want=%q", id, rule.DocsRef, expectedURL)
		}

		docPath := filepath.Join(docsDir, id+".md")
		content, err := os.ReadFile(docPath)
		if err != nil {
			missing = append(missing, id)
			continue
		}
		if !strings.Contains(string(content), "# "+id) {
			t.Fatalf("docs page %s must include heading '# %s'", docPath, id)
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("missing docs pages for rules: %s", strings.Join(missing, ", "))
	}
}
