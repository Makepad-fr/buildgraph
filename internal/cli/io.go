package cli

import "io"

// IO groups stdio streams for CLI operations.
type IO struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}
