package cli

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestRunUnknownCommandReturnsUsage(t *testing.T) {
	t.Parallel()
	app, err := NewApp(IO{In: strings.NewReader(""), Out: io.Discard, Err: io.Discard})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	code := app.Run(context.Background(), []string{"unknown"})
	if code != ExitUsage {
		t.Fatalf("expected usage exit code, got %d", code)
	}
}

func TestParseGlobalFlagsFromCommandTail(t *testing.T) {
	t.Parallel()
	opts, remaining, err := parseGlobalFlags([]string{"analyze", "--json", "--verbose"}, io.Discard)
	if err != nil {
		t.Fatalf("parse global flags: %v", err)
	}
	if !opts.JSON || !opts.Verbose {
		t.Fatalf("expected json and verbose to be true, got json=%t verbose=%t", opts.JSON, opts.Verbose)
	}
	if len(remaining) != 1 || remaining[0] != "analyze" {
		t.Fatalf("unexpected remaining args: %v", remaining)
	}
}
