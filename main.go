package main

import (
	"context"
	"fmt"
	"os"
	_ "time/tzdata"

	"github.com/tgragnato/orbiter/internal/startup"
)

// GitRev is injected by the compiler via -ldflags.
var GitRev string

func main() {
	ctx := context.Background()
	if err := startup.Run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  orbiter [--dsn <dsn>]                              start TUI")
	fmt.Fprintln(os.Stderr, "  orbiter backup  [--dsn <dsn>] [--output <file>]   dump transactions to JSON")
	fmt.Fprintln(os.Stderr, "  orbiter restore [--dsn <dsn>] [--input  <file>]   load transactions from JSON")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "DSN can also be provided via DATABASE_URL environment variable.")
	fmt.Fprintln(os.Stderr, "Default output/input file: transactions.json")
}
