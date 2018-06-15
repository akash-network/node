package initgen

import tmtypes "github.com/tendermint/tendermint/types"

type Context interface {
	Name() string
	Path() string

	Nodes() []*Node
	Genesis() *tmtypes.GenesisDoc
}

func NewContext(name, path string, genesis *tmtypes.GenesisDoc, nodes ...*Node) Context {
	return context{
		name:    name,
		path:    path,
		genesis: genesis,
		nodes:   nodes,
	}
}

type context struct {
	name    string
	path    string
	genesis *tmtypes.GenesisDoc
	nodes   []*Node
}

func (ctx context) Name() string {
	return ctx.name
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
