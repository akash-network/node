package marketplace

import (
	"fmt"

	"github.com/tendermint/tendermint/libs/pubsub"
	tmquery "github.com/tendermint/tendermint/libs/pubsub/query"
	tmtypes "github.com/tendermint/tendermint/types"
)

// tmquery "github.com/tendermint/tendermint/libs/pubsub/query"
// tmtypes "github.com/tendermint/tendermint/types"

func TxQuery() pubsub.Query {
	return tmquery.MustParse(
		fmt.Sprintf("%s='%s'", tmtypes.EventTypeKey, tmtypes.EventTx))
}

func BlkQuery() pubsub.Query {
	return tmquery.MustParse(
		fmt.Sprintf("%s='%s'", tmtypes.EventTypeKey, tmtypes.EventNewBlockHeader))
}

// func TxQuery() pubsub.Query {

// 	return tmquery.Empty{}
// 	// return tmquery.MustParse(fmt.Sprintf("( %s EXISTS", tmtypes.EventTypeKey))

// 	// return tmquery.MustParse(fmt.Sprintf("%s='%s' OR %s='%s' OR %s='%s'",
// 	// return tmquery.MustParse(fmt.Sprintf("%s='%s'")) // tmtypes.EventTypeKey, tmtypes.EventTx,
// 	// tmtypes.EventTypeKey, tmtypes.EventNewBlock,
// 	// tmtypes.EventTypeKey, tmtypes.EventNewBlockHeader,

// }
