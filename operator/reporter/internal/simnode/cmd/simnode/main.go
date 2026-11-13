// Command simnode serves a named simnode scenario for processes outside Go tests: the TS parity goldens and the
// steady-100k benchmark.
//
//	simnode list
//	simnode serve --scenario NAME [--akash-latency D] [--solana-latency D] [--block-fetch-latency D]
//
// serve prints one JSON line with the node's endpoints and the reporter configuration the scenario expects, then
// serves until stdin closes or the process is interrupted.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"pkg.akt.dev/node/v2/operator/reporter/internal/simnode"
)

const usage = "usage: simnode list | simnode serve --scenario NAME " +
	"[--akash-latency D] [--solana-latency D] [--block-fetch-latency D]"

// ready is the line serve prints once the node is listening.
type ready struct {
	Comet  string `json:"comet"`
	GRPC   string `json:"grpc"`
	Solana string `json:"solana"`
	// Passes is how many passes the goldens run, at least one.
	Passes         int     `json:"passes"`
	ChainID        string  `json:"chainId"`
	Reporter       string  `json:"reporter"`
	GenesisHash    string  `json:"genesisHash"`
	ProgramID      string  `json:"programId"`
	Mint           string  `json:"mint"`
	MaxAgeSeconds  int     `json:"maxAgeSeconds"`
	LightClientPin *string `json:"lightClientPin,omitempty"`
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}

	switch args[0] {
	case "list":
		for _, name := range slices.Sorted(maps.Keys(simnode.Scenarios())) {
			if _, err := fmt.Fprintln(stdout, name); err != nil {
				return err
			}
		}

		return nil
	case "serve":
		return serve(args[1:], stdin, stdout)
	default:
		return errors.New(usage)
	}
}

func serve(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	name := fs.String("scenario", "", "scenario name, from simnode list")
	var opts simnode.Options
	fs.DurationVar(&opts.AkashLatency, "akash-latency", 0, "delay of every CometBFT and gRPC request")
	fs.DurationVar(&opts.SolanaLatency, "solana-latency", 0, "delay of every Solana request")
	fs.DurationVar(&opts.BlockFetchLatency, "block-fetch-latency", 0, "further delay of every getBlock request")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New(usage)
	}

	build, ok := simnode.Scenarios()[*name]
	if !ok {
		return fmt.Errorf("unknown scenario %q; see simnode list", *name)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sc := build()
	n, err := simnode.Start(ctx, sc, opts)
	if err != nil {
		return err
	}
	defer n.Close()

	line := ready{
		Comet:         n.CometURL,
		GRPC:          n.GRPCAddr,
		Solana:        n.SolanaURL,
		Passes:        sc.PassCount(),
		ChainID:       simnode.ChainID,
		Reporter:      simnode.Reporter,
		GenesisHash:   simnode.GenesisHash,
		ProgramID:     simnode.ProgramID.String(),
		Mint:          simnode.MintAddress.String(),
		MaxAgeSeconds: int(simnode.ReporterMaxAge / time.Second),
	}
	if sc.LightClientPin != nil {
		pin := sc.LightClientPin.String()
		line.LightClientPin = &pin
	}
	if err := json.NewEncoder(stdout).Encode(line); err != nil {
		return err
	}

	closed := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, stdin)
		close(closed)
	}()

	select {
	case <-closed:
	case <-ctx.Done():
	}

	return nil
}
