package initgen

import (
	"encoding/json"
	"fmt"

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

// NewBuilder returns a new instance of the builder
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

// Create generates a configuration context
func (b *builder) Create() (Context, error) {

	// Generate FilePVs
	pvkeys := b.generateFilePVKeys()
	// Generate node keys
	nodekeys := b.generateNodeKeys()
	// Generate nodes
	nodes := b.generateNodes(pvkeys, nodekeys)

	// Extract public keys from filePVs
	pubkeys := make([]crypto.PubKey, 0, b.count)
	for i := uint(0); i < b.count; i++ {
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

	return NewContext(b.name, b.path, genesis, nodes...), nil
}

func (b *builder) generatePrivateValidators() []tmtypes.PrivValidator {

	if b.count == 0 {
		return nil
	}

	validators := make([]tmtypes.PrivValidator, 0, b.count)
	for i := uint(0); i < b.count; i++ {
		validators = append(validators, privval.GenFilePV("", ""))
	}
	return validators
}

func (b *builder) generateFilePVKeys() []*privval.FilePV {
	if b.count == 0 {
		return nil
	}

	filepvkeys := make([]*privval.FilePV, 0, b.count)
	for i := uint(0); i < b.count; i++ {
		filepvkeys = append(filepvkeys, privval.GenFilePV("", ""))
	}
	return filepvkeys
}

func (b *builder) generateNodeKeys() []*p2p.NodeKey {
	if b.count == 0 {
		return nil
	}
	keys := make([]*p2p.NodeKey, 0, b.count)
	for i := uint(0); i < b.count; i++ {
		key := &p2p.NodeKey{PrivKey: ed25519.GenPrivKey()}
		keys = append(keys, key)
	}
	return keys
}

func (b *builder) generateNodes(fvals []*privval.FilePV, nodekeys []*p2p.NodeKey) []*Node {
	if b.count == 0 {
		return nil
	}

	nodes := make([]*Node, 0, b.count)

	if b.count == 1 {
		return []*Node{
			{Name: b.name, FilePV: fvals[0], NodeKey: nodekeys[0]}}
	}

	for n := uint(0); n < b.count; n++ {
		nodes = append(nodes, &Node{
			Name:    fmt.Sprintf("%v-%v", b.name, n),
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
			Name:    fmt.Sprintf("%v-%v", b.name, idx),
			PubKey:  pk,
			Address: pk.Address(),
			Power:   10,
		})
	}

	return gvalidators
}
