package types

import (
	tmtypes "github.com/tendermint/abci/types"
)

const (
	TagNameApp    = "app"
	TagNameTxType = "tx.type"

	TagAppAccount = "account"
	TxTypeSend    = "send"

	TagAppDeployment = "deployment"
	TxTypeDeployment = "deployment"

	TagAppDeploymentOrder       = "deployment-order"
	TxTypeCreateDeploymentOrder = "deployment-order-create"

	TagAppDatacenter       = "datacenter"
	TxTypeDatacenterCreate = "datacenter-create"
)

func NewTagApp(name string) *tmtypes.KVPair {
	return tmtypes.KVPairString(TagNameApp, name)
}

func NewTagTxType(name string) *tmtypes.KVPair {
	return tmtypes.KVPairString(TagNameTxType, name)
}

func NewTags(appName, txType string) []*tmtypes.KVPair {
	return []*tmtypes.KVPair{
		NewTagApp(appName),
		NewTagTxType(txType),
	}
}
