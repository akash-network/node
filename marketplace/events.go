package marketplace

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	abci_types "github.com/tendermint/tendermint/abci/types"
)

func unmarshalLeaseCreate(evt abci_types.Event) (*types.TxCreateLease, error) {
	ev := &types.TxCreateLease{}
	found := 0

	for _, attr := range evt.GetAttributes() {
		if bytes.Equal(attr.GetKey(), []byte("lease-id")) {
			id, err := keys.ParseLeasePath(string(attr.GetValue()))
			if err != nil {
				return nil, err
			}
			ev.LeaseID = id.LeaseID
			found++
		}
		if bytes.Equal(attr.GetKey(), []byte("price")) {
			val, err := strconv.ParseUint(string(attr.GetValue()), 10, 64)
			if err != nil {
				return nil, err
			}
			ev.Price = val
			found++
		}
	}

	if found == 2 {
		return ev, nil
	}

	return nil, fmt.Errorf("invalid lease-create")
}

func unmarshalLeaseClose(evt abci_types.Event) (*types.TxCloseLease, error) {
	for _, attr := range evt.GetAttributes() {
		if bytes.Equal(attr.GetKey(), []byte("lease-id")) {
			id, err := keys.ParseLeasePath(string(attr.GetValue()))
			if err != nil {
				return nil, err
			}
			return &types.TxCloseLease{
				LeaseID: id.LeaseID,
			}, nil
		}
	}
	return nil, fmt.Errorf("invalid lease-create")
}

func unmarshalOrderCreate(evt abci_types.Event) (*types.TxCreateOrder, error) {
	for _, attr := range evt.GetAttributes() {
		if bytes.Equal(attr.GetKey(), []byte("order-id")) {
			path := string(attr.GetValue())
			id, err := keys.ParseOrderPath(path)
			if err != nil {
				return nil, err
			}
			return &types.TxCreateOrder{
				OrderID: id.OrderID,
			}, nil
		}
	}
	return nil, fmt.Errorf("invalid order-create")
}
