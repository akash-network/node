package initgen

import (
	"encoding/json"
	"fmt"

	"github.com/ovrclk/akash/types"
	crypto "github.com/tendermint/go-crypto"
	"github.com/tendermint/tendermint/p2p"
	tmtypes "github.com/tendermint/tendermint/types"
	privval "github.com/tendermint/tendermint/types/priv_validator"
)

const (
	chainID = "local"
)

type Node struct {
	Name             string
	PrivateValidator tmtypes.PrivValidator
	NodeKey          *p2p.NodeKey
	Peers            []*Node
}

type Builder interface {
	WithName(string) Builder
	WithPath(string) Builder
	WithCount(uint) Builder
	WithAkashGenesis(*types.Genesis) Builder
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

func (b *builder) WithAkashGenesis(pgenesis *types.Genesis) Builder {
	b.pgenesis = pgenesis
	return b
}

func (b *builder) Create() (Context, error) {

	pvalidators := b.generatePrivateValidators()
	validators := b.generateValidators(pvalidators)
	nodekeys := b.generateNodeKeys()
	nodes := b.generateNodes(pvalidators, nodekeys)

	genesis := &tmtypes.GenesisDoc{
		ChainID:    chainID,
		Validators: validators,
	}

	if b.pgenesis != nil {
		buf, err := json.Marshal(b.pgenesis)
		if err != nil {
			return nil, err
		}
		genesis.AppOptions = buf
	}

	if err := genesis.ValidateAndComplete(); err != nil {
		return nil, err
	}

	return NewContext(b.name, b.path, genesis, nodes...), nil
}

func (b *builder) generatePrivateValidators() []tmtypes.PrivValidator {

	if b.count == 0 {
		return nil
	}

	validators := make([]tmtypes.PrivValidator, 0, b.count)

	for i := uint(0); i < b.count; i++ {
		validators = append(validators, privval.GenFilePV(""))
	}
	return validators
}

func (b *builder) generateNodeKeys() []*p2p.NodeKey {
	if b.count == 0 {
		return nil
	}
	keys := make([]*p2p.NodeKey, 0, b.count)
	for i := uint(0); i < b.count; i++ {
		key := &p2p.NodeKey{PrivKey: crypto.GenPrivKeyEd25519()}
		keys = append(keys, key)
	}
	return keys
}

func (b *builder) generateNodes(pvals []tmtypes.PrivValidator, nodekeys []*p2p.NodeKey) []*Node {
	if b.count == 0 {
		return nil
	}

	nodes := make([]*Node, 0, b.count)

	if b.count == 1 {
		return []*Node{
			{Name: b.name, PrivateValidator: pvals[0], NodeKey: nodekeys[0]},
		}
	}

	for n := uint(0); n < b.count; n++ {
		nodes = append(nodes, &Node{
			Name:             fmt.Sprintf("%v-%v", b.name, n),
			PrivateValidator: pvals[n],
			NodeKey:          nodekeys[n],
		})
	}

	for n, node := range nodes {
		for i := 0; i < len(nodes); i++ {
			if n != i {
				node.Peers = append(node.Peers, nodes[i])
			}
		}
	}

	return nodes
}

func (b *builder) generateValidators(pvalidators []tmtypes.PrivValidator) []tmtypes.GenesisValidator {

	if len(pvalidators) == 1 {
		return []tmtypes.GenesisValidator{
			{
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
