package node

import (
	"encoding/json"
	"errors"
	"io/ioutil"

	"github.com/ovrclk/photon/types"
	tmtypes "github.com/tendermint/tendermint/types"
)

// Tendermint genesis doc from file
func TMGenesisFromFile(path string) (*tmtypes.GenesisDoc, error) {
	obj := tmtypes.GenesisDoc{
		AppOptions: &types.Genesis{},
	}

	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(buf, &obj); err != nil {
		return nil, err
	}

	return &obj, nil
}

// Photon genesis doc from file
func GenesisFromTMGenesis(genesis *tmtypes.GenesisDoc) (*types.Genesis, error) {
	obj, ok := genesis.AppOptions.(*types.Genesis)
	if ok {
		return obj, nil
	}
	return nil, errors.New("invalid genesis")
}
