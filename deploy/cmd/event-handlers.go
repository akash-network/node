package cmd

import (
	"context"
	"fmt"
	"github.com/avast/retry-go"
	"github.com/spf13/viper"
	"net/http"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	gateway "github.com/ovrclk/akash/provider/gateway/rest"
	"github.com/ovrclk/akash/pubsub"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/pkg/errors"
)

// EventHandler is a type of function that handles events coming out of the event bus
type EventHandler func(pubsub.Event) error

// SendManifestHander sends manifests on the lease created event
func SendManifestHander(clientCtx client.Context, dd *DeploymentData, gClientDir *gateway.ClientDirectory, retryConfiguration []retry.Option) func(pubsub.Event) error {
	pollingRate := viper.GetDuration(FlagTick)
	retryIf := func(err error) bool {
		isGatewayError := retryIfGatewayClientResponseError(err)
		if isGatewayError {
			gwError := err.(gateway.ClientResponseError)

			switch gwError.Status {
			case http.StatusInternalServerError:
				return false // don't retry, the provider can't use this manifest
			case http.StatusUnprocessableEntity:
				return false // don't retry, the manifest isn't well formed
			default:
			}

		}

		return isGatewayError
	}

	var localRetryConfiguration []retry.Option
	localRetryConfiguration = append(localRetryConfiguration, retryConfiguration...)
	localRetryConfiguration = append(localRetryConfiguration, retry.RetryIf(retryIf))

	return func(ev pubsub.Event) (err error) {
		addr := clientCtx.GetFromAddress()
		log := logger.With("action", "send-manifest")

		evLeaseCreated, ok := ev.(mtypes.EventLeaseCreated)
		if ok && addr.String() == evLeaseCreated.ID.Owner && evLeaseCreated.ID.DSeq == dd.DeploymentID.DSeq {
			// The provider responds to the same event to get ready for a deployment, so sleep here to
			// avoid racing the provider
			time.Sleep(pollingRate)
			log.Info("sending manifest to provider", "provider", evLeaseCreated.ID.Provider, "dseq", evLeaseCreated.ID.DSeq)

			gclient, err := gClientDir.GetClientFromBech32(evLeaseCreated.ID.Provider)
			if err != nil {
				return err
			}

			return retry.Do(func() error {
				err := gclient.SubmitManifest(context.Background(), dd.DeploymentID.DSeq, dd.Manifest)
				if err != nil {
					log.Debug("send-manifest failed", "lease", evLeaseCreated.ID, "err", err)
				}

				return err
			}, localRetryConfiguration...)
		}
		return
	}
}

var errUnexpectedEvent = errors.New("unexpected event")

// DeploymentDataUpdateHandler updates a DeploymentData and prints relevant events
func DeploymentDataUpdateHandler(dd *DeploymentData, bids chan<- mtypes.EventBidCreated, leasesReady chan<- struct{}) func(pubsub.Event) error {
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
				bids <- event
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
				if dd.ExpectedLeases() {
					// Write to channel without blocking, it is buffered
					select {
					case leasesReady <- struct{}{}:
						log.Info("All expected leases created")
					default:
					}
				}

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

		// Ignore any other event
		default:
			log.Debug("Ignoring event")
			return
		}
	}
}
