package deploy

import (
	ccontext "context"
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client/context"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/client/utils"
	"github.com/ovrclk/akash/events"
	"github.com/ovrclk/akash/provider/gateway"
	"github.com/ovrclk/akash/provider/manifest"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/sdl"
	dclient "github.com/ovrclk/akash/x/deployment/client/cli"
	"github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
	pmodule "github.com/ovrclk/akash/x/provider"
)

// CMD returns the command for deploy
func CMD(cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [sdl-file]",
		Short: fmt.Sprintf("Create a deployment, listen for order clearing and ensure provider has manifest"),
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
			id, err := dclient.DeploymentIDFromFlags(cmd.Flags(), ctx.GetFromAddress().String())
			if err != nil {
				return err
			}

			// Default DSeq to the current block height
			if id.DSeq == 0 {
				if id.DSeq, err = dclient.CurrentBlockHeight(ctx); err != nil {
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

			msg := types.NewMsgCreateDeployment(id, groups)
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
	dclient.AddDeploymentIDFlags(cmd.Flags())

	return cmd
}
