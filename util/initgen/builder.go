package initgen

import (
	"encoding/json"

	"github.com/ovrclk/akash/types"
	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/privval"
	tmtypes "github.com/tendermint/tendermint/types"
	tmtime "github.com/tendermint/tendermint/types/time"
)

const (
	chainID = "local"
)

// Node represents an Akash node
type Node struct {
	Name string
	//FilePV is file pvs
	FilePV  *privval.FilePV
	NodeKey *p2p.NodeKey
	Peers   []*Node
}

// Builder is the config builder
type Builder interface {
	WithNames([]string) Builder
	WithPath(string) Builder
	WithAkashGenesis(*types.Genesis) Builder
	Create() (Context, error)
}

type builder struct {
	names    []string
	path     string
	pgenesis *types.Genesis
	type_    Type
}

// NewBuilder returns a new instance of the builder
func NewBuilder() Builder {
	return &builder{}
}

func (b *builder) WithNames(names []string) Builder {
	b.names = names
	return b
}

func (b *builder) WithPath(path string) Builder {
	b.path = path
	return b
}

func (b *builder) WithAkashGenesis(pgenesis *types.Genesis) Builder {
	b.pgenesis = pgenesis
	return b
}

// Create generates a configuration context
func (b *builder) Create() (Context, error) {

	// Generate FilePVs
	pvkeys := b.generateFilePVKeys()
	// Generate node keys
	nodekeys := b.generateNodeKeys()
	// Generate nodes
	nodes := b.generateNodes(pvkeys, nodekeys)

	// Extract public keys from filePVs
	pubkeys := make([]crypto.PubKey, 0, len(b.names))
	for i := 0; i < len(b.names); i++ {
		pubkeys = append(pubkeys, pvkeys[i].Key.PubKey)
	}
	// make public keys as validators in genesis doc
	validators := b.generateValidators(pubkeys)
	genesis := &tmtypes.GenesisDoc{
		GenesisTime: tmtime.Now(),
		ChainID:     chainID,
		Validators:  validators,
	}
	// specify the account with balances in genesis
	if b.pgenesis != nil {
		buf, err := json.Marshal(b.pgenesis)
		if err != nil {
			return nil, err
		}
		genesis.AppState = buf
	}

	if err := genesis.ValidateAndComplete(); err != nil {
		return nil, err
	}

	return NewContext(b.path, genesis, nodes...), nil
}

func (b *builder) generatePrivateValidators() []tmtypes.PrivValidator {

	if len(b.names) == 0 {
		return nil
	}

	validators := make([]tmtypes.PrivValidator, 0, len(b.names))
	for i := 0; i < len(b.names); i++ {
		validators = append(validators, privval.GenFilePV("", ""))
	}
	return validators
}

func (b *builder) generateFilePVKeys() []*privval.FilePV {
	if len(b.names) == 0 {
		return nil
	}

	filepvkeys := make([]*privval.FilePV, 0, len(b.names))
	for i := 0; i < len(b.names); i++ {
		filepvkeys = append(filepvkeys, privval.GenFilePV("", ""))
	}
	return filepvkeys
}

func (b *builder) generateNodeKeys() []*p2p.NodeKey {
	if len(b.names) == 0 {
		return nil
	}
	keys := make([]*p2p.NodeKey, 0, len(b.names))
	for i := 0; i < len(b.names); i++ {
		key := &p2p.NodeKey{PrivKey: ed25519.GenPrivKey()}
		keys = append(keys, key)
	}
	return keys
}

func (b *builder) generateNodes(fvals []*privval.FilePV, nodekeys []*p2p.NodeKey) []*Node {
	if len(b.names) == 0 {
		return nil
	}

	nodes := make([]*Node, 0, len(b.names))

	if len(b.names) == 1 {
		return []*Node{
			{Name: b.names[0], FilePV: fvals[0], NodeKey: nodekeys[0]}}
	}

	for n := 0; n < len(b.names); n++ {
		nodes = append(nodes, &Node{
			Name:    b.names[n],
			NodeKey: nodekeys[n],
			FilePV:  fvals[n],
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

func (b *builder) generateValidators(pubKeys []crypto.PubKey) []tmtypes.GenesisValidator {
	if len(pubKeys) == 1 {
		return []tmtypes.GenesisValidator{
			{
				Power:   10,
				PubKey:  pubKeys[0],
				Address: pubKeys[0].Address(),
			},
		}
	}

	var gvalidators []tmtypes.GenesisValidator

	for idx, pk := range pubKeys {
		gvalidators = append(gvalidators, tmtypes.GenesisValidator{
			Name:    b.names[idx],
			PubKey:  pk,
			Address: pk.Address(),
			Power:   10,
		})
	}

	return gvalidators
}
