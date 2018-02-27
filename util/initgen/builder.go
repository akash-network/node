package initgen

import (
	"fmt"

	"github.com/ovrclk/photon/types"
	tmtypes "github.com/tendermint/tendermint/types"
)

const (
	chainID = "local"
)

type Builder interface {
	WithName(string) Builder
	WithPath(string) Builder
	WithCount(uint) Builder
	WithPhotonGenesis(*types.Genesis) Builder
	Create() (Context, error)
}

type builder struct {
	name     string
	path     string
	count    uint
	pgenesis *types.Genesis
	type_    Type
}

func NewBuilder() Builder {
	return &builder{}
}

func (b *builder) WithName(name string) Builder {
	b.name = name
	return b
}

func (b *builder) WithPath(path string) Builder {
	b.path = path
	return b
}

func (b *builder) WithCount(count uint) Builder {
	b.count = count
	return b
}

func (b *builder) WithPhotonGenesis(pgenesis *types.Genesis) Builder {
	b.pgenesis = pgenesis
	return b
}

func (b *builder) Create() (Context, error) {

	pvalidators := b.generatePrivateValidators()
	validators := b.generateValidators(pvalidators)

	genesis := &tmtypes.GenesisDoc{
		ChainID:    chainID,
		Validators: validators,
		AppOptions: b.pgenesis,
	}

	if err := genesis.ValidateAndComplete(); err != nil {
		return nil, err
	}

	return NewContext(b.name, b.path, genesis, pvalidators...), nil
}

func (b *builder) generatePrivateValidators() []tmtypes.PrivValidator {

	if b.count == 0 {
		return nil
	}

	validators := make([]tmtypes.PrivValidator, 0, b.count)

	for i := uint(0); i < b.count; i++ {
		validators = append(validators, tmtypes.GenPrivValidatorFS(""))
	}
	return validators
}

func (b *builder) generateValidators(pvalidators []tmtypes.PrivValidator) []tmtypes.GenesisValidator {

	if len(pvalidators) == 1 {
		return []tmtypes.GenesisValidator{
			tmtypes.GenesisValidator{
				Name:   b.name,
				Power:  10,
				PubKey: pvalidators[0].GetPubKey(),
			},
		}
	}

	var gvalidators []tmtypes.GenesisValidator

	for idx, gv := range pvalidators {
		gvalidators = append(gvalidators, tmtypes.GenesisValidator{
			Name:   fmt.Sprintf("%v-%v", b.name, idx),
			Power:  10,
			PubKey: gv.GetPubKey(),
		})
	}

	return gvalidators
}
