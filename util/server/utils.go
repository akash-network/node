package server

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"cosmossdk.io/log"
	"golang.org/x/sync/errgroup"
)

// ListenForQuitSignals listens for SIGINT and SIGTERM. When a signal is received,
// the cleanup function is called, indicating the caller can gracefully exit or
// return.
//
// Note, the blocking behavior of this depends on the block argument.
// The caller must ensure the corresponding context derived from the cancelFn is used correctly.
func ListenForQuitSignals(ctx context.Context, cancelFn context.CancelFunc, g *errgroup.Group, block bool, logger log.Logger) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	f := func() {
		select {
		case sig := <-sigCh:
			logger.Info("caught signal", "signal", sig.String())
		case <-ctx.Done():
			logger.Info("context canceled")
		}

		cancelFn()
	}

	if block {
		g.Go(func() error {
			f()
			return nil
		})
	} else {
		go f()
	}
}
