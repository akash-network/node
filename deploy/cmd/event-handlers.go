package cmd

import (
	"context"
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/ovrclk/akash/provider/gateway"
	"github.com/ovrclk/akash/provider/manifest"
	"github.com/ovrclk/akash/pubsub"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
	pmodule "github.com/ovrclk/akash/x/provider"
	ptypes "github.com/ovrclk/akash/x/provider/types"
	"github.com/pkg/errors"
)

var (
	errUnreachableCode = errors.New("should be unreachable code, exit with this error")
)

// EventHandler is a type of function that handles events coming out of the event bus
type EventHandler func(pubsub.Event) error

// SendManifestHander sends manifests on the lease created event
func SendManifestHander(clientCtx client.Context, dd *DeploymentData) func(pubsub.Event) error {
	return func(ev pubsub.Event) (err error) {
		addr := clientCtx.GetFromAddress()
		log := logger.With("action", "send-manifest")
		switch event := ev.(type) {
		// Handle Lease creation events
		case mtypes.EventLeaseCreated:
			if addr.String() == event.ID.Owner && event.ID.DSeq == dd.DeploymentID.DSeq {
				pclient := pmodule.AppModuleBasic{}.GetQueryClient(clientCtx)
				res, err := pclient.Provider(
					context.Background(),
					&ptypes.QueryProviderRequest{Owner: event.ID.Provider},
				)
				if err != nil {
					return err
				}

				provider := res.Provider

				log.Info("sending manifest to provider", "provider", event.ID.Provider, "uri", provider.HostURI, "dseq", event.ID.DSeq)
				if err = gateway.NewClient().SubmitManifest(
					context.Background(),
					provider.HostURI,
					&manifest.SubmitRequest{
						Deployment: event.ID.DeploymentID(),
						Manifest:   dd.Manifest,
					},
				); err != nil {
					return err
				}
			}
		}
		return
	}
}

var errUnexpectedEvent = errors.New("unexpected event")

// DeploymentDataUpdateHandler updates a DeploymentData and prints relevant events
func DeploymentDataUpdateHandler(dd *DeploymentData) func(pubsub.Event) error {
	return func(ev pubsub.Event) (err error) {
		addr := dd.DeploymentID.Owner
		log := logger.With("addr", addr, "dseq", dd.DeploymentID.DSeq)
		switch event := ev.(type) {
		// Handle deployment creation events
		case dtypes.EventDeploymentCreated:
			if event.ID.Equals(dd.DeploymentID) {
				log.Info("deployment created")
			}
			return

		// Handle deployment update events
		case dtypes.EventDeploymentUpdated:
			if event.ID.Equals(dd.DeploymentID) {
				log.Info("deployment updated")
			}
			return

		// Handle deployment close events
		case dtypes.EventDeploymentClosed:
			if event.ID.Equals(dd.DeploymentID) {
				log.Error("deployment closed unexpectedly")

				// TODO - exit here
				return fmt.Errorf("%w: deployment closed", errUnexpectedEvent)
			}
			return

		// Handle deployment group close events
		case dtypes.EventGroupClosed:
			if event.ID.Owner == addr && event.ID.DSeq == dd.DeploymentID.DSeq {
				// TODO: Maybe more housekeeping here?
				log.Info("deployment group closed")
			}
			return

		// Handle Order creation events
		case mtypes.EventOrderCreated:
			if addr == event.ID.Owner && event.ID.DSeq == dd.DeploymentID.DSeq {
				dd.AddOrder(event.ID)
				log.Info("order for deployment created", "oseq", event.ID.OSeq)
			}
			return

		// Handle Order close events
		case mtypes.EventOrderClosed:
			if addr == event.ID.Owner && event.ID.DSeq == dd.DeploymentID.DSeq {
				dd.RemoveOrder(event.ID)
				log.Info("order for deployment closed", "oseq", event.ID.OSeq)
			}
			return

		// Handle Bid creation events
		case mtypes.EventBidCreated:
			if addr == event.ID.Owner && event.ID.DSeq == dd.DeploymentID.DSeq {
				log.Info("bid for order created", "oseq", event.ID.OSeq, "price", event.Price)
			}
			return

		// Handle Bid close events
		case mtypes.EventBidClosed:
			if addr == event.ID.Owner && event.ID.DSeq == dd.DeploymentID.DSeq {
				log.Info("bid for order closed", "oseq", event.ID.OSeq, "price", event.Price)
			}
			return

		// Handle Lease creation events
		case mtypes.EventLeaseCreated:
			if addr == event.ID.Owner && event.ID.DSeq == dd.DeploymentID.DSeq {
				dd.AddLease(event.ID)
				log.Info("lease for order created", "oseq", event.ID.OSeq, "price", event.Price)
			}
			return

		// Handle Lease close events
		case mtypes.EventLeaseClosed:
			if addr == event.ID.Owner && event.ID.DSeq == dd.DeploymentID.DSeq {
				log.Error("lease for order closed", "oseq", event.ID.OSeq, "price", event.Price)
				return fmt.Errorf("%w: lease closed oseq: %v", errUnexpectedEvent, event.ID.OSeq)
			}
			return

		// In any other case we should exit with error
		default:
			return fmt.Errorf("%w: %T", errUnexpectedEvent, ev)
		}
	}
}
