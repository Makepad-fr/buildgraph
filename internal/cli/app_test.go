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

func TestNormalizeProgressMode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		value      string
		globalJSON bool
		want       string
		wantErr    bool
	}{
		{name: "auto human", value: "auto", globalJSON: false, want: "human"},
		{name: "auto json", value: "auto", globalJSON: true, want: "none"},
		{name: "explicit json", value: "json", globalJSON: false, want: "json"},
		{name: "invalid", value: "wat", globalJSON: false, wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeProgressMode(tc.value, tc.globalJSON)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("normalize progress mode: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected mode: got=%q want=%q", got, tc.want)
			}
		})
	}
}
