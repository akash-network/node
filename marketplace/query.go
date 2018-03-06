package marketplace

import (
	"fmt"

	apptypes "github.com/ovrclk/photon/app/types"
	tmtmtypes "github.com/tendermint/tendermint/types"
	"github.com/tendermint/tmlibs/pubsub"
	tmquery "github.com/tendermint/tmlibs/pubsub/query"
)

func TxQuery() pubsub.Query {
	return buildTxQuery("")
}

func TxQueryApp(name string) pubsub.Query {
	return buildTxQuery("%s='%s'", apptypes.TagNameApp, name)
}

func TxQueryTxType(name string) pubsub.Query {
	return buildTxQuery("%s='%s'", apptypes.TagNameTxType, name)
}

func TxQueryCreateDeploymentOrder() pubsub.Query {
	return TxQueryTxType(apptypes.TxTypeCreateDeploymentOrder)
}

func buildTxQuery(format string, args ...interface{}) pubsub.Query {
	val := fmt.Sprintf("%s='%s'", tmtmtypes.EventTypeKey, tmtmtypes.EventTx)
	if format != "" {
		val += fmt.Sprintf(" AND "+format, args...)
	}
	return tmquery.MustParse(val)
}
