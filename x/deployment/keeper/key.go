package keeper

import (
	"bytes"
	"encoding/binary"

	"github.com/ovrclk/akash/x/deployment/types"
)

var (
	deploymentPrefix = []byte{0x01}

	groupPrefix = []byte{0x02}
	// groupOpenPrefix is used only to track the set of Groups in Open state which need to have orders assigned.
	groupOpenPrefix = []byte{0x03}
)

func deploymentKey(id types.DeploymentID) []byte {
	buf := bytes.NewBuffer(deploymentPrefix)
	buf.Write([]byte(id.Owner))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// groupKey provides prefixed key for a Group's marshalled data.
func groupKey(id types.GroupID) []byte {
	buf := bytes.NewBuffer(groupPrefix)
	buf.Write([]byte(id.Owner))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	if err := binary.Write(buf, binary.BigEndian, id.GSeq); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// groupOpenKey provides prefixed key for groups which are in open state.
// No data is stored under the key.
func groupOpenKey(id types.GroupID) []byte {
	buf := bytes.NewBuffer(groupOpenPrefix)
	buf.Write([]byte(id.Owner))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	if err := binary.Write(buf, binary.BigEndian, id.GSeq); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// groupOpenKeyConvert converts an open key to the original
// group key prefix for accessing the Group's data.
func groupOpenKeyConvert(openKey []byte) ([]byte, error) {
	buf := bytes.NewBuffer(groupPrefix)
	_, err := buf.Write(openKey[len(groupPrefix):])
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// groupsKey provides default store Key for Group data.
func groupsKey(id types.DeploymentID) []byte {
	buf := bytes.NewBuffer(groupPrefix)
	buf.Write([]byte(id.Owner))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
