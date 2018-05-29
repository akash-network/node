package event

import (
	"context"

	"github.com/ovrclk/akash/marketplace"
	"github.com/ovrclk/akash/types"
	tmtmtypes "github.com/tendermint/tendermint/types"
	"github.com/tendermint/tmlibs/log"
)

type (
	// Transactions needed for provider services.  May not be necessary - they
	// originally had more data/functionality but it was removed for simplicity.

	TxCreateOrder       = types.TxCreateOrder
	TxCreateFulfillment = types.TxCreateFulfillment
	TxCreateLease       = types.TxCreateLease
	TxCloseDeployment   = types.TxCloseDeployment
	TxCloseFulfillment  = types.TxCloseFulfillment
)

// Wrap tendermint event bus - publish events from tendermint bus to our bus implementation.
func MarketplaceTxPublisher(ctx context.Context, log log.Logger, tmbus tmtmtypes.EventBusSubscriber, bus Bus) (marketplace.Monitor, error) {
	handler := MarketplaceTxHandler(bus)
	return marketplace.NewMonitor(ctx, log, tmbus, "tx-publisher", handler, marketplace.TxQuery())
}

func MarketplaceTxHandler(bus Bus) marketplace.Handler {
	return marketplace.NewBuilder().
		OnTxCreateOrder(func(tx *types.TxCreateOrder) {
			bus.Publish((*TxCreateOrder)(tx))
		}).
		OnTxCreateFulfillment(func(tx *types.TxCreateFulfillment) {
			bus.Publish((*TxCreateFulfillment)(tx))
		}).
		OnTxCreateLease(func(tx *types.TxCreateLease) {
			bus.Publish((*TxCreateLease)(tx))
		}).
		OnTxCloseDeployment(func(tx *types.TxCloseDeployment) {
			bus.Publish((*TxCloseDeployment)(tx))
		}).
		OnTxCloseFulfillment(func(tx *types.TxCloseFulfillment) {
			bus.Publish((*TxCloseFulfillment)(tx))
		}).
		Create()
}
