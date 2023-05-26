package events

import (
	"fmt"

	"github.com/cometbft/cometbft/libs/pubsub"
	tmquery "github.com/cometbft/cometbft/libs/pubsub/query"
	tmtypes "github.com/cometbft/cometbft/types"
)

func txQuery() pubsub.Query {
	return tmquery.MustParse(
		fmt.Sprintf("%s='%s'", tmtypes.EventTypeKey, tmtypes.EventTx))
}

func blkQuery() pubsub.Query {
	return tmquery.MustParse(
		fmt.Sprintf("%s='%s'", tmtypes.EventTypeKey, tmtypes.EventNewBlockHeader))
}
