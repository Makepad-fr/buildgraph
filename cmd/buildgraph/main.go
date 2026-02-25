package main

import (
	"context"
	"os"

	"github.com/Makepad-fr/buildgraph/internal/cli"
)

func main() {
	app, err := cli.NewApp(cli.IO{In: os.Stdin, Out: os.Stdout, Err: os.Stderr})
	if err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(cli.ExitInternal)
	}
	os.Exit(app.Run(context.Background(), os.Args[1:]))
}
