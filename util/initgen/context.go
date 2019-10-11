package initgen

import tmtypes "github.com/tendermint/tendermint/types"

type Context interface {
	Path() string

	Nodes() []*Node
	Genesis() *tmtypes.GenesisDoc
}

func NewContext(path string, genesis *tmtypes.GenesisDoc, nodes ...*Node) Context {
	return context{
		path:    path,
		genesis: genesis,
		nodes:   nodes,
	}
}

type context struct {
	path    string
	genesis *tmtypes.GenesisDoc
	nodes   []*Node
}

func (ctx context) Path() string {
	return ctx.path
}

func (ctx context) Nodes() []*Node {
	return ctx.nodes
}

func (ctx context) Genesis() *tmtypes.GenesisDoc {
	return ctx.genesis
}
