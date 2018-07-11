package event

import (
	"context"

	"github.com/ovrclk/akash/marketplace"
	"github.com/ovrclk/akash/types"
	"github.com/tendermint/tendermint/libs/log"
	tmtmtypes "github.com/tendermint/tendermint/types"
)

type (
	// Transactions needed for provider services.  May not be necessary - they
	// originally had more data/functionality but it was removed for simplicity.

	TxCreateOrder       = types.TxCreateOrder
	TxCreateFulfillment = types.TxCreateFulfillment
	TxCreateLease       = types.TxCreateLease
	TxUpdateDeployment  = types.TxUpdateDeployment
	TxCloseDeployment   = types.TxCloseDeployment
	TxCloseFulfillment  = types.TxCloseFulfillment
	TxCloseLease        = types.TxCloseLease
)

// Wrap tendermint event bus - publish events from tendermint bus to our bus implementation.
func MarketplaceTxPublisher(ctx context.Context, log log.Logger, tmbus tmtmtypes.EventBusSubscriber, bus Bus) (marketplace.Monitor, error) {
	handler := MarketplaceTxHandler(bus)
	return marketplace.NewMonitor(ctx, log, tmbus, "tx-publisher", handler, marketplace.TxQuery())
}

func MarketplaceTxHandler(bus Bus) marketplace.Handler {
	return marketplace.NewBuilder().
		OnTxCreateOrder(func(tx *types.TxCreateOrder) {
			bus.Publish(tx)
		}).
		OnTxCreateFulfillment(func(tx *types.TxCreateFulfillment) {
			bus.Publish(tx)
		}).
		OnTxCreateLease(func(tx *types.TxCreateLease) {
			bus.Publish(tx)
		}).
		OnTxUpdateDeployment(func(tx *types.TxUpdateDeployment) {
			bus.Publish(tx)
		}).
		OnTxCloseDeployment(func(tx *types.TxCloseDeployment) {
			bus.Publish(tx)
		}).
		OnTxCloseFulfillment(func(tx *types.TxCloseFulfillment) {
			bus.Publish(tx)
		}).
		OnTxCloseLease(func(tx *types.TxCloseLease) {
			bus.Publish(tx)
		}).
		Create()
}
