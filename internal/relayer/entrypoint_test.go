package relayer

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWatcherFinishesActiveTickOnStop(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	entered, finish := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	deps := defaultDependencies(Streams{})
	deps.sleep = func(context.Context, time.Duration) error {
		t.Error("watcher slept after its stop request")
		return nil
	}
	go func() {
		done <- runTicks(ctx, true, time.Second, func(work context.Context) error {
			close(entered)
			<-finish
			return work.Err()
		}, deps)
	}()
	<-entered
	stop()
	select {
	case err := <-done:
		t.Fatalf("watcher abandoned active work: %v", err)
	default:
	}
	close(finish)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not finish after its active tick")
	}
}

func TestWatcherRetriesFailureButStopsDuringIdle(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	var output bytes.Buffer
	deps := defaultDependencies(Streams{
		Err: &output,
	})
	waits, ticks := 0, 0
	deps.sleep = func(waitContext context.Context, delay time.Duration) error {
		waits++
		if delay != 7*time.Second {
			t.Fatalf("wrong poll interval: %s", delay)
		}
		if waits == 2 {
			stop()
			return waitContext.Err()
		}
		return nil
	}
	err := runTicks(ctx, true, 7*time.Second, func(context.Context) error {
		ticks++
		return errors.New("retained intent awaiting history")
	}, deps)
	if err != nil || ticks != 2 || waits != 2 {
		t.Fatalf("watch result: %v, ticks=%d, waits=%d", err, ticks, waits)
	}
	if strings.Count(output.String(), "retained intent awaiting history") != 2 {
		t.Fatal("watch failures were hidden")
	}
}

func TestOneShotReturnsFailureWithoutRetry(t *testing.T) {
	want := errors.New("RPC unavailable")
	calls := 0
	err := runTicks(context.Background(), false, time.Second, func(context.Context) error {
		calls++
		return want
	}, defaultDependencies(Streams{}))
	if !errors.Is(err, want) || calls != 1 {
		t.Fatalf("one shot returned %v after %d attempts", err, calls)
	}
}

func TestWatcherStopsBeforeFirstTick(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	stop()
	ticks := 0
	err := runTicks(ctx, true, time.Second, func(context.Context) error {
		ticks++
		return nil
	}, defaultDependencies(Streams{}))
	if err != nil || ticks != 0 {
		t.Fatalf("stopped watcher returned %v after %d ticks", err, ticks)
	}
}

func TestWatcherStopsBetweenIdleWaitAndNextTick(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	ticks := 0
	deps := defaultDependencies(Streams{})
	deps.sleep = func(context.Context, time.Duration) error {
		stop()
		return nil
	}
	err := runTicks(ctx, true, time.Second, func(context.Context) error {
		ticks++
		return nil
	}, deps)
	if err != nil || ticks != 1 {
		t.Fatalf("idle stop returned %v after %d ticks", err, ticks)
	}
}

func TestPublicCLIHelpIsOffline(t *testing.T) {
	var output bytes.Buffer
	if err := Run(context.Background(), []string{"--help"}, BuildInfo{}, Streams{
		Out: &output,
	}); err != nil {
		t.Fatal(err)
	}
	if output.String() != helpText {
		t.Fatal("public help differs from documented commands")
	}
}

func TestLocalDiagnosticParsing(t *testing.T) {
	for _, args := range [][]string{
		{"local", "update", "123", "--tamper"},
		{"local", "relay", "id", "123"},
	} {
		if _, err := parseCommand(args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	for _, args := range [][]string{
		{"run", "--config", "/config", "--tamper"},
		{"local", "--tamper"},
		{"local", "tick", "--tamper"},
		{"local", "--bootstrap", "update", "123", "--tamper"},
		{"local", "relay", "id", "123", "extra"},
	} {
		if _, err := parseCommand(args); err == nil {
			t.Fatalf("invalid diagnostic accepted: %v", args)
		}
	}
	for _, height := range []string{"0", "-1", "+1", "01", "9007199254740992"} {
		if _, err := parseHeight(height); err == nil {
			t.Fatalf("invalid height accepted: %s", height)
		}
	}
}
