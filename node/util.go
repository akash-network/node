package node

import (
	"encoding/json"
	"io/ioutil"

	"github.com/ovrclk/akash/types"
	tmtypes "github.com/tendermint/tendermint/types"
)

// Tendermint genesis doc from file
func TMGenesisFromFile(path string) (*tmtypes.GenesisDoc, error) {
	obj := new(tmtypes.GenesisDoc)

	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(buf, obj); err != nil {
		return nil, err
	}

	return obj, nil
}

// Akash genesis doc from file
func GenesisFromTMGenesis(genesis *tmtypes.GenesisDoc) (*types.Genesis, error) {
	obj := new(types.Genesis)
	if err := json.Unmarshal(genesis.AppOptions, obj); err != nil {
		return nil, err
	}
	return obj, nil
}
