package cli

import "context"

// Command abstracts a runnable CLI command.
type Command interface {
	Name() string
	Run(ctx context.Context, args []string) error
}
