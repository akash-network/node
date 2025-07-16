package cmd

import (
	"context"
	"errors"

	"github.com/cosmos/cosmos-sdk/client"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"

	cflags "pkg.akt.dev/go/cli/flags"

	cmdcommon "pkg.akt.dev/node/cmd/common"
	"pkg.akt.dev/node/events"
	"pkg.akt.dev/node/pubsub"
)

// EventCmd prints out events in real time
func EventCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "events",
		Short: "Prints out akash events in real time",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdcommon.RunForeverWithContext(cmd.Context(), func(ctx context.Context) error {
				return getEvents(ctx, cmd, args)
			})
		},
	}

	cmd.Flags().String(cflags.FlagNode, "tcp://localhost:26657", "The node address")
	if err := viper.BindPFlag(cflags.FlagNode, cmd.Flags().Lookup(cflags.FlagNode)); err != nil {
		return nil
	}

	return cmd
}

func getEvents(ctx context.Context, cmd *cobra.Command, _ []string) error {
	cctx := client.GetClientContextFromCmd(cmd)

	node, err := cctx.GetNode()
	if err != nil {
		return err
	}

	bus := pubsub.NewBus()
	defer bus.Close()

	group, ctx := errgroup.WithContext(ctx)

	subscriber, err := bus.Subscribe()
	if err != nil {
		return err
	}

	evtSvc, err := events.NewEvents(ctx, node, "akash-cli", bus)
	if err != nil {
		return err
	}

	group.Go(func() error {
		<-ctx.Done()
		evtSvc.Shutdown()

		return nil
	})

	group.Go(func() error {
		for {
			select {
			case <-subscriber.Done():
				return nil
			case ev := <-subscriber.Events():
				if err := cmdcommon.PrintJSON(cctx, ev); err != nil {
					return err
				}
			}
		}
	})

	err = group.Wait()
	if err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}
