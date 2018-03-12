package types

import (
	tmcommon "github.com/tendermint/tmlibs/common"
)

const (
	TagNameApp        = "app"
	TagNameTxType     = "tx.type"
	TagNameDeployment = "market.deployment"
	TagNameLease      = "market.lease"

	TagAppAccount = "account"
	TxTypeSend    = "send"

	TagAppDeployment = "deployment"
	TxTypeDeployment = "deployment"

	TagAppDeploymentOrder       = "deployment-order"
	TxTypeCreateDeploymentOrder = "deployment-order-create"

	TagAppFulfillmentOrder       = "fulfillment-order"
	TxTypeCreateFulfillmentOrder = "fulfillment-order-create"

	TagAppLease       = "lease"
	TxTypeCreateLease = "lease-create"

	TagAppDatacenter       = "datacenter"
	TxTypeDatacenterCreate = "datacenter-create"
)

func NewTagApp(name string) tmcommon.KVPair {
	return kvPair(TagNameApp, name)
}

func NewTagTxType(name string) tmcommon.KVPair {
	return kvPair(TagNameTxType, name)
}

func NewTags(appName, txType string) []tmcommon.KVPair {
	return []tmcommon.KVPair{
		NewTagApp(appName),
		NewTagTxType(txType),
	}
}

func kvPair(k, v string) tmcommon.KVPair {
	return tmcommon.KVPair{
		Key:   []byte(k),
		Value: []byte(v),
	}
}
