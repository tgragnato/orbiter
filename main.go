package main

import (
	"context"
	"fmt"
	"os"

	"github.com/sklinkert/at/internal/app"
)

// GitRev is injected by the compiler via -ldflags.
var GitRev string

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]
	ctx := context.Background()
	var err error
	switch command {
	case "backtesting":
		err = app.RunBacktesting(ctx, GitRev, args)
	case "at-ig":
		err = app.RunIG(ctx, GitRev, args)
	case "at-coinbase":
		err = app.RunCoinbase(ctx, GitRev, args)
	case "import-histdata":
		err = app.RunImportHistdata(ctx, args)
	case "help", "-h", "--help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", command)
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: at <command>")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  backtesting      Run the historical backtester")
	fmt.Fprintln(os.Stderr, "  at-ig            Run the IG.com live trading setup")
	fmt.Fprintln(os.Stderr, "  at-coinbase      Run the Coinbase trading setup")
	fmt.Fprintln(os.Stderr, "  import-histdata  Import HistData CSV files into PostgreSQL")
}
