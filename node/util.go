package node

import (
	"io/ioutil"
	"os"

	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/privval"
	tmtypes "github.com/tendermint/tendermint/types"
)

// TMGenesisFromFile returns tendermint genesis doc from file
func TMGenesisFromFile(path string) (*tmtypes.GenesisDoc, error) {
	obj := new(tmtypes.GenesisDoc)

	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := cdc.UnmarshalJSON(buf, obj); err != nil {
		return nil, err
	}

	return obj, nil
}

// TMGenesisToJSON converts tendermint genesis doc to json.
// Returns error in case of failure.
func TMGenesisToJSON(obj *tmtypes.GenesisDoc) ([]byte, error) {
	return cdc.MarshalJSON(obj)
}

// FilePVFromFile reads data from provided filepath and returns filePV.
// Returns error in case of failure.
func FilePVFromFile(path string) (*privval.FilePV, error) {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return FilePVFromJSON(buf)
}

// FilePVFromJSON returns FilePV from json. Returns error in case of failure.
func FilePVFromJSON(buf []byte) (*privval.FilePV, error) {
	obj := new(privval.FilePV)
	return obj, cdc.UnmarshalJSON(buf, obj)
}

// FilePVToJSON converts FilePV to json. Returns error in case of failure.
func FilePVToJSON(obj *privval.FilePV) ([]byte, error) {
	return cdc.MarshalJSON(obj)
}

// FilePVKeyFromJSON returns FilePVKey from json. Returns error in case of failure.
func FilePVKeyFromJSON(buf []byte) (*privval.FilePVKey, error) {
	obj := new(privval.FilePVKey)
	return obj, cdc.UnmarshalJSON(buf, obj)
}

// FilePVKeyToJSON converts FilePVKey to json. Returns error in case of failure.
func FilePVKeyToJSON(obj privval.FilePVKey) ([]byte, error) {
	return cdc.MarshalJSON(obj)
}

// FilePVStateToJSON converts FilePVState to json
func FilePVStateToJSON(obj privval.FilePVLastSignState) ([]byte, error) {
	return cdc.MarshalJSON(obj)
}

// PVKeyToFile writes PV key to a file with given filemode permissions.
// Written File is saved into path provided. Returns error in case of failure.
func PVKeyToFile(path string, perm os.FileMode, obj privval.FilePVKey) error {
	return writeConfigIfNotExist(path, perm, obj)
}

// PVStateToFile writes PV state to a file with given filemode permissions.
// Written File is saved into path provided. Returns error in case of failure.
func PVStateToFile(path string, perm os.FileMode, obj privval.FilePVLastSignState) error {
	return writeConfigIfNotExist(path, perm, obj)
}

// NodeKeyToJSON converts node key to json. Returns error in case of failure.
func NodeKeyToJSON(obj *p2p.NodeKey) ([]byte, error) {
	return cdc.MarshalJSON(obj)
}

// NodeKeyToFile writes node key to a file with given filemode permissions.
// Written File is saved into path provided. Returns error in case of failure.
func NodeKeyToFile(path string, perm os.FileMode, obj *p2p.NodeKey) error {
	return writeConfigIfNotExist(path, perm, obj)
}

func writeConfigIfNotExist(path string, perm os.FileMode, obj interface{}) error {
	data, err := cdc.MarshalJSONIndent(obj, "", "  ")
	if err != nil {
		return err
	}
	_, err = os.Stat(path)
	if !os.IsNotExist(err) {
		return nil
	}
	err = ioutil.WriteFile(path, data, perm)
	if err != nil {
		return err
	}
	return nil
}
