package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"pkg.akt.dev/node/v2/internal/relayer"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := relayer.Run(ctx, os.Args[1:], relayer.BuildInfo{
		Version:  version,
		Commit:   commit,
		Platform: runtime.GOOS + "_" + runtime.GOARCH,
	}, relayer.Streams{
		In:  os.Stdin,
		Out: os.Stdout,
		Err: os.Stderr,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
