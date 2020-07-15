package cmd

import (
	"context"

	ccontext "github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"

	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/events"
	"github.com/ovrclk/akash/pubsub"
)

// EventCmd prints out events in real time
func EventCmd(cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "events",
		Short: "Prints out akash events in real time",
		RunE: func(cmd *cobra.Command, args []string) error {
			return common.RunForever(func(ctx context.Context) error {
				return getEvents(ctx, cdc, cmd, args)
			})
		},
	}

	cmd.Flags().String(flags.FlagNode, "", "The node address")
	_ = viper.BindPFlag(flags.FlagNode, cmd.Flags().Lookup(flags.FlagNode))

	return cmd
}

func getEvents(ctx context.Context, cdc *codec.Codec, _ *cobra.Command, _ []string) error {
	cctx := ccontext.NewCLIContext().WithCodec(cdc)

	if err := cctx.Client.Start(); err != nil {
		return err
	}

	bus := pubsub.NewBus()
	defer bus.Close()

	group, ctx := errgroup.WithContext(ctx)

	subscriber, err := bus.Subscribe()

	if err != nil {
		return err
	}

	group.Go(func() error {
		return events.Publish(ctx, cctx.Client, "akash-cli", bus)
	})

	group.Go(func() error {
		for {
			select {
			case <-subscriber.Done():
				return nil
			case ev := <-subscriber.Events():
				cctx.PrintOutput(ev)
			}
		}
	})

	return group.Wait()
}
