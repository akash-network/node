package events

import (
	"fmt"

	"github.com/tendermint/tendermint/libs/pubsub"
	tmquery "github.com/tendermint/tendermint/libs/pubsub/query"
	tmtypes "github.com/tendermint/tendermint/types"
)

func txQuery() pubsub.Query {
	return tmquery.MustParse(
		fmt.Sprintf("%s='%s'", tmtypes.EventTypeKey, tmtypes.EventTx))
}

func blkQuery() pubsub.Query {
	return tmquery.MustParse(
		fmt.Sprintf("%s='%s'", tmtypes.EventTypeKey, tmtypes.EventNewBlockHeader))
}
