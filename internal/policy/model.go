package policy

// Rule describes a built-in or externally provided policy check.
type Rule struct {
	ID         string `json:"id"`
	Dimension  string `json:"dimension"`
	Severity   string `json:"severity"`
	Matcher    string `json:"matcher"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
	DocsRef    string `json:"docsRef"`
}
