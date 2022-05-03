package cmd

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	dockerterm "github.com/moby/term"
	akashclient "github.com/ovrclk/akash/client"
	gwrest "github.com/ovrclk/akash/provider/gateway/rest"
	cutils "github.com/ovrclk/akash/x/cert/utils"
	dcli "github.com/ovrclk/akash/x/deployment/client/cli"
	mcli "github.com/ovrclk/akash/x/market/client/cli"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/util/term"
)

const (
	FlagStdin        = "stdin"
	FlagTty          = "tty"
	FlagReplicaIndex = "replica-index"
)

var errTerminalNotATty = errors.New("Input is not a terminal, cannot setup TTY")

func LeaseShellCmd() *cobra.Command {
	cmd := &cobra.Command{
		Args:         cobra.MinimumNArgs(2),
		Use:          "lease-shell",
		Short:        "do lease shell",
		SilenceUsage: true,
		RunE:         doLeaseShell,
	}

	addLeaseFlags(cmd)
	cmd.Flags().Bool(FlagStdin, false, "connect stdin")
	if err := viper.BindPFlag(FlagStdin, cmd.Flags().Lookup(FlagStdin)); err != nil {
		return nil
	}

	cmd.Flags().Bool(FlagTty, false, "connect an interactive terminal")
	if err := viper.BindPFlag(FlagTty, cmd.Flags().Lookup(FlagTty)); err != nil {
		return nil
	}

	cmd.Flags().Uint(FlagReplicaIndex, 0, "replica index to connect to")
	if err := viper.BindPFlag(FlagReplicaIndex, cmd.Flags().Lookup(FlagReplicaIndex)); err != nil {
		return nil
	}

	return cmd
}

func doLeaseShell(cmd *cobra.Command, args []string) error {
	var stdin io.ReadCloser
	var stdout io.Writer
	var stderr io.Writer
	stdout = os.Stdout
	stderr = os.Stderr
	connectStdin := viper.GetBool(FlagStdin)
	setupTty := viper.GetBool(FlagTty)
	podIndex := viper.GetUint(FlagReplicaIndex)
	if connectStdin || setupTty {
		stdin = os.Stdin
	}

	var tty term.TTY
	var tsq remotecommand.TerminalSizeQueue
	if setupTty {
		tty = term.TTY{
			Parent: nil,
			Out:    os.Stdout,
			In:     stdin,
		}

		if !tty.IsTerminalIn() {
			return errTerminalNotATty
		}

		dockerStdin, dockerStdout, _ := dockerterm.StdStreams()

		tty.In = dockerStdin
		tty.Out = dockerStdout

		stdin = dockerStdin
		stdout = dockerStdout
		tsq = tty.MonitorSize(tty.GetSize())
		tty.Raw = true
	}

	cctx, err := sdkclient.GetClientTxContext(cmd)
	if err != nil {
		return err
	}

	prov, err := providerFromFlags(cmd.Flags())
	if err != nil {
		return err
	}

	bidID, err := mcli.BidIDFromFlags(cmd.Flags(), dcli.WithOwner(cctx.FromAddress))
	if err != nil {
		return err
	}
	lID := bidID.LeaseID()

	cert, err := cutils.LoadAndQueryCertificateForAccount(cmd.Context(), cctx, nil)
	if err != nil {
		return markRPCServerError(err)
	}

	gclient, err := gwrest.NewClient(akashclient.NewQueryClientFromCtx(cctx), prov, []tls.Certificate{cert})
	if err != nil {
		return err
	}

	service := args[0]
	remoteCmd := args[1:]

	var terminalResizes chan remotecommand.TerminalSize
	wg := &sync.WaitGroup{}
	ctx, cancel := context.WithCancel(cmd.Context())

	if tsq != nil {
		terminalResizes = make(chan remotecommand.TerminalSize, 1)
		go func() {
			for {
				// this blocks waiting for a resize event, the docs suggest
				// that this isn't the case but there is not a code path that ever does that
				// so this goroutine is just left running until the process exits
				size := tsq.Next()
				if size == nil {
					return
				}
				terminalResizes <- *size

			}
		}()
	}

	signals := make(chan os.Signal, 1)
	signalsToCatch := []os.Signal{syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGHUP}
	if !setupTty { // if the terminal is not interactive, handle SIGINT
		signalsToCatch = append(signalsToCatch, syscall.SIGINT)
	}
	signal.Notify(signals, signalsToCatch...)
	wasHalted := make(chan os.Signal, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case sig := <-signals:
			cancel()
			wasHalted <- sig
		case <-ctx.Done():
		}
	}()
	leaseShellFn := func() error {
		return gclient.LeaseShell(ctx, lID, service, podIndex, remoteCmd, stdin, stdout, stderr, setupTty, terminalResizes)
	}

	if setupTty { // Interactive terminals run with a wrapper that restores the prior state
		err = tty.Safe(leaseShellFn)
	} else {
		err = leaseShellFn()
	}

	// Check if a signal halted things
	select {
	case haltSignal := <-wasHalted:
		_ = cctx.PrintString(fmt.Sprintf("\nhalted by signal: %v\n", haltSignal))
		err = nil // Don't show this error, as it is always something complaining about use of a closed connection
	default:
		cancel()
	}
	wg.Wait()

	if err != nil {
		return showErrorToUser(err)
	}
	return nil
}
