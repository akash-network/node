package types

import (
	abci_types "github.com/tendermint/tendermint/abci/types"
	tmcommon "github.com/tendermint/tendermint/libs/common"
)

const (
	TagNameApp        = "app"
	TagNameTxType     = "tx.type"
	TagNameDeployment = "market.deployment"
	TagNameLease      = "market.lease"

	TagAppAccount = "account"
	TxTypeSend    = "send"

	TagAppDeployment       = "deployment"
	TxTypeCreateDeployment = "deployment-create"
	TxTypeUpdateDeployment = "deployment-update"
	TxTypeCloseDeployment  = "deployment-close"

	TagAppOrder       = "order"
	TxTypeCreateOrder = "order-create"

	TagAppFulfillment       = "fulfillment"
	TxTypeCreateFulfillment = "fulfillment-create"
	TxTypeCloseFulfillment  = "fulfillment-close"

	TagAppLease       = "lease"
	TxTypeCreateLease = "lease-create"
	TxTypeCloseLease  = "lease-close"

	TagAppProvider       = "provider"
	TxTypeProviderCreate = "provider-create"
)

func Events(appName, txType string, attrs ...tmcommon.KVPair) []abci_types.Event {
	return []abci_types.Event{
		{
			Type:       txType,
			Attributes: append(attrs, newTagApp(appName)),
		},
	}
}

func newTagApp(name string) tmcommon.KVPair {
	return kvPair(TagNameApp, name)
}

func kvPair(k, v string) tmcommon.KVPair {
	return tmcommon.KVPair{
		Key:   []byte(k),
		Value: []byte(v),
	}
}
