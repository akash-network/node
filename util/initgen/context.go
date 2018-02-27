package initgen

import tmtypes "github.com/tendermint/tendermint/types"

type Context interface {
	Name() string
	Path() string

	PrivateValidators() []tmtypes.PrivValidator
	Genesis() *tmtypes.GenesisDoc
}

func NewContext(name, path string, genesis *tmtypes.GenesisDoc, pvalidators ...tmtypes.PrivValidator) Context {
	return context{
		name:        name,
		path:        path,
		genesis:     genesis,
		pvalidators: pvalidators,
	}
}

type context struct {
	name        string
	path        string
	genesis     *tmtypes.GenesisDoc
	pvalidators []tmtypes.PrivValidator
}

func (ctx context) Name() string {
	return ctx.name
}

func (ctx context) Path() string {
	return ctx.path
}

func (ctx context) PrivateValidators() []tmtypes.PrivValidator {
	return ctx.pvalidators
}

func (ctx context) Genesis() *tmtypes.GenesisDoc {
	return ctx.genesis
}
