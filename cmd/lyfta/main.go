package main

import (
	"context"
	"os"
	"os/signal"

	"lyftacli/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(cli.New(os.Stdin, os.Stdout, os.Stderr).Execute(ctx, os.Args[1:]))
}
