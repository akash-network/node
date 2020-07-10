package cli

import (
	ccontext "context"
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"golang.org/x/sync/errgroup"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/client/utils"
	"github.com/ovrclk/akash/events"
	"github.com/ovrclk/akash/provider/gateway"
	"github.com/ovrclk/akash/provider/manifest"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
	pmodule "github.com/ovrclk/akash/x/provider"

	"github.com/spf13/cobra"
)

// GetTxCmd returns the transaction commands for this module
func GetTxCmd(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Deployment transaction subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(flags.PostCommands(
		cmdCreate(key, cdc),
		cmdUpdate(key, cdc),
		cmdClose(key, cdc),
		cmdGroupClose(key, cdc),
		makeItSoCommand(key, cdc),
	)...)
	return cmd
}

func makeItSoCommand(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "make-it-so [sdl-file]",
		Short: fmt.Sprintf("Create %s", key),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Setup CLI context and transaction builder
			ctx := context.NewCLIContext().WithCodec(cdc)
			bldr := auth.NewTxBuilderFromCLI(os.Stdin).WithTxEncoder(utils.GetTxEncoder(cdc))
			if err := ctx.Client.Start(); err != nil {
				return err
			}

			// Read in manifest file
			sdl, err := sdl.ReadFile(args[0])
			if err != nil {
				return err
			}

			// Fetch deployment groups from the SDL file, each group maps to an individual order
			groups, err := sdl.DeploymentGroups()
			if err != nil {
				return err
			}

			// Fetch the manifest from the SDL file
			mani, err := sdl.Manifest()
			if err != nil {
				return err
			}

			// Get the deployment ID (defaults to height)
			id, err := DeploymentIDFromFlags(cmd.Flags(), ctx.GetFromAddress().String())
			if err != nil {
				return err
			}

			// Default DSeq to the current block height
			if id.DSeq == 0 {
				if id.DSeq, err = currentBlockHeight(ctx); err != nil {
					return err
				}
			}

			// create a new pubsub bus to handle chain events
			bus := pubsub.NewBus()
			defer bus.Close()

			// Create an error group to handle chain listener
			group, cctx := errgroup.WithContext(ccontext.Background())
			cctx, cancel := ccontext.WithCancel(cctx)

			// Listen to the events on chain and publish them to the pubsub bus
			group.Go(func() error {
				return events.Publish(cctx, ctx.Client, "deployment-create", bus)
			})

			// expose event channel
			sub, err := bus.Subscribe()
			if err != nil {
				return err
			}

			// start goroutine to listen for events
			leases := len(groups)
			group.Go(func() error {
				for {
					select {
					case <-sub.Done():
						return nil
					case ev := <-sub.Events():
						switch msg := ev.(type) {
						case types.EventDeploymentCreated:
							fmt.Printf("Deployment %d created...\n", msg.ID.DSeq)
						case mtypes.EventOrderCreated:
							fmt.Printf("Order %d for deployement %d created...\n", msg.ID.OSeq, msg.ID.DSeq)
						case mtypes.EventBidCreated:
							fmt.Printf("Bid of %s for order %d:%d created...\n", msg.Price, msg.ID.DSeq, msg.ID.OSeq)
						case mtypes.EventLeaseCreated:
							fmt.Printf("Lease for order %d:%d created...\n", msg.ID.DSeq, msg.ID.OSeq)
							pclient := pmodule.AppModuleBasic{}.GetQueryClient(ctx)
							provider, err := pclient.Provider(msg.ID.Provider)
							if err != nil {
								return err
							}

							gclient := gateway.NewClient()

							fmt.Printf("Sending manifest to provider %s...\n", msg.ID.Provider)
							if err = gclient.SubmitManifest(
								ccontext.Background(),
								provider.HostURI,
								&manifest.SubmitRequest{
									Deployment: msg.ID.DeploymentID(),
									Manifest:   mani,
								},
							); err != nil {
								return err
							}

							if leases = leases - 1; leases == 0 {
								fmt.Printf("Leases left %d...\n", leases)
								sub.Close()
								cancel()
								return nil
							}
						}
					}
				}
			})

			// Create the deployment message
			msg := types.MsgCreateDeployment{
				ID: id,
				// Version:  []byte{0x1, 0x2},
				Groups: make([]types.GroupSpec, 0, len(groups)),
			}

			// Append the groups to the message
			for _, group := range groups {
				msg.Groups = append(msg.Groups, *group)
			}

			// validate
			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			// broadcast the message
			// TODO: set transaction broadcast defaults on the ctx to ensure
			// that GenerateOrBroadcastMsgs exits when tx is sent to chain
			if err = utils.GenerateOrBroadcastMsgs(ctx, bldr, []sdk.Msg{msg}); err != nil {
				return err
			}

			return group.Wait()
		},
	}
	AddDeploymentIDFlags(cmd.Flags())

	return cmd
}

func cmdCreate(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [sdl-file]",
		Short: fmt.Sprintf("Create %s", key),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)
			bldr := auth.NewTxBuilderFromCLI(os.Stdin).WithTxEncoder(utils.GetTxEncoder(cdc))

			sdl, err := sdl.ReadFile(args[0])
			if err != nil {
				return err
			}

			groups, err := sdl.DeploymentGroups()
			if err != nil {
				return err
			}

			id, err := DeploymentIDFromFlags(cmd.Flags(), ctx.GetFromAddress().String())
			if err != nil {
				return err
			}

			// Default DSeq to the current block height
			if id.DSeq == 0 {
				if id.DSeq, err = currentBlockHeight(ctx); err != nil {
					return err
				}
			}

			msg := types.MsgCreateDeployment{
				ID: id,
				// Version:  []byte{0x1, 0x2},
				Groups: make([]types.GroupSpec, 0, len(groups)),
			}

			for _, group := range groups {
				msg.Groups = append(msg.Groups, *group)
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return utils.GenerateOrBroadcastMsgs(ctx, bldr, []sdk.Msg{msg})
		},
	}
	AddDeploymentIDFlags(cmd.Flags())

	return cmd
}

func cmdClose(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "close",
		Short: fmt.Sprintf("Close %s", key),
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)
			bldr := auth.NewTxBuilderFromCLI(os.Stdin).WithTxEncoder(utils.GetTxEncoder(cdc))

			id, err := DeploymentIDFromFlags(cmd.Flags(), ctx.GetFromAddress().String())
			if err != nil {
				return err
			}

			msg := types.MsgCloseDeployment{ID: id}

			return utils.GenerateOrBroadcastMsgs(ctx, bldr, []sdk.Msg{msg})
		},
	}
	AddDeploymentIDFlags(cmd.Flags())
	return cmd
}

func cmdUpdate(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [sdl-file]",
		Short: fmt.Sprintf("update %s", key),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)
			bldr := auth.NewTxBuilderFromCLI(os.Stdin).WithTxEncoder(utils.GetTxEncoder(cdc))

			id, err := DeploymentIDFromFlags(cmd.Flags(), ctx.GetFromAddress().String())
			if err != nil {
				return err
			}

			msg := types.MsgUpdateDeployment{
				ID: id,
			}

			return utils.GenerateOrBroadcastMsgs(ctx, bldr, []sdk.Msg{msg})
		},
	}
	AddDeploymentIDFlags(cmd.Flags())
	return cmd
}

func cmdGroupClose(_ string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "group-close",
		Short:   "close a Deployment's specific Group",
		Example: "akashctl tx deployment group-close --owner=[Account Address] --dseq=[uint64] --gseq=[uint32]",
		Args:    cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)
			bldr := auth.NewTxBuilderFromCLI(os.Stdin).WithTxEncoder(utils.GetTxEncoder(cdc))
			id, err := GroupIDFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			msg := types.MsgCloseGroup{
				ID: id,
			}
			err = msg.ValidateBasic()
			if err != nil {
				return err
			}

			return utils.GenerateOrBroadcastMsgs(ctx, bldr, []sdk.Msg{msg})
		},
	}
	AddGroupIDFlags(cmd.Flags())
	MarkReqGroupIDFlags(cmd)

	return cmd
}
