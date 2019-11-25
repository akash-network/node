package market

import (
	"strconv"

	"github.com/ovrclk/akash/types"
	abci_types "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/common"
)

// TODO: merge this with parsing in marketplace/events.go

func eventLeaseClose(lease *types.Lease) abci_types.Event {
	return abci_types.Event{
		Type: "market.lease-close",
		Attributes: []common.KVPair{
			{Key: []byte("lease-id"), Value: []byte(lease.LeaseID.String())},
			{Key: []byte("reason"), Value: []byte(types.TxCloseDeployment_INSUFFICIENT.String())},
		},
	}
}

func eventLeaseCreate(lease *types.Lease) abci_types.Event {
	return abci_types.Event{
		Type: "market.lease-create",
		Attributes: []common.KVPair{
			{Key: []byte("lease-id"), Value: []byte(lease.LeaseID.String())},
			{Key: []byte("price"), Value: []byte(strconv.FormatUint(lease.Price, 10))},
		},
	}
}

func eventOrderCreate(order *types.Order) abci_types.Event {
	return abci_types.Event{
		Type: "market.order-create",
		Attributes: []common.KVPair{
			{Key: []byte("order-id"), Value: []byte(order.OrderID.String())},
		},
	}
}
