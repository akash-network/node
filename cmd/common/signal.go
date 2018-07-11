package common

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func WatchSignals(ctx context.Context, cancel context.CancelFunc) <-chan struct{} {
	donech := make(chan struct{})
	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM)
	go func() {
		defer close(donech)
		defer signal.Stop(sigch)
		select {
		case <-ctx.Done():
		case <-sigch:
			cancel()
		}
	}()
	return donech
}

func RunForever(fn func(ctx context.Context) error) error {
	return RunForeverWithContext(context.Background(), fn)
}

func RunForeverWithContext(ctx context.Context, fn func(ctx context.Context) error) error {
	ctx, cancel := context.WithCancel(ctx)

	donech := WatchSignals(ctx, cancel)

	err := fn(ctx)

	cancel()
	<-donech

	return err
}
