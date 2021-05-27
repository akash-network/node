package cmd

import (
	"crypto/tls"
	"errors"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	dockerterm "github.com/moby/term"
	akashclient "github.com/ovrclk/akash/client"
	cmdcommon "github.com/ovrclk/akash/cmd/common"
	gwrest "github.com/ovrclk/akash/provider/gateway/rest"
	cutils "github.com/ovrclk/akash/x/cert/utils"
	mcli "github.com/ovrclk/akash/x/market/client/cli"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/util/term"
	"os"
	"sync"
)

const (
	FlagStdin = "stdin"
	FlagTty = "tty"
)

func leaseShellCmd() *cobra.Command {
	cmd := &cobra.Command{
		Args: cobra.MinimumNArgs(1),
		Use:          "lease-shell",
		Short:        "do lease shell",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doLeaseShell(cmd, args)
		},
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
	if connectStdin || setupTty {
		stdin = os.Stdin
	}

	var tty term.TTY
	var tsq remotecommand.TerminalSizeQueue
	if setupTty {
		tty = term.TTY{
			Parent: nil,
			Out: os.Stdout,
			In: stdin,
		}

		if ! tty.IsTerminalIn() {
			return errors.New("Input is not a terminal, cannot setup TTY")
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

	bid, err := mcli.BidIDFromFlagsForOwner(cmd.Flags(), cctx.FromAddress)
	if err != nil {
		return err
	}

	cert, err := cutils.LoadAndQueryCertificateForAccount(cmd.Context(), cctx, cctx.Keyring)
	if err != nil {
		return err
	}

	gclient, err := gwrest.NewClient(akashclient.NewQueryClientFromCtx(cctx), prov, []tls.Certificate{cert})
	if err != nil {
		return err
	}

	service := args[0]
	remoteCmd := args[1:]

	var terminalResizes chan remotecommand.TerminalSize
	wg := &sync.WaitGroup{}
	if tsq != nil {
		terminalResizes = make(chan remotecommand.TerminalSize, 1)
		wg.Add(1)
		go func () {
			defer wg.Done()
			for {
				size := tsq.Next() // this blocks waiting for a resize event
				if size == nil {   // this means reisze events have ended
					close(terminalResizes)
					return
				}

				terminalResizes <- *size
			}
		}()
	}

	leaseShellFn := func() error {
		return gclient.LeaseShell(cmd.Context(), bid.LeaseID(), service, remoteCmd, stdin, stdout, stderr, setupTty, terminalResizes)
	}
	if setupTty {
		err = tty.Safe(leaseShellFn)
	} else {
		err = leaseShellFn()
	}
	if err != nil {
		return showErrorToUser(err)
	}

	return cmdcommon.PrintJSON(cctx, "done")
}

