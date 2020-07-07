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
	buf.Write(id.Owner.Bytes())
	binary.Write(buf, binary.BigEndian, id.DSeq)
	return buf.Bytes()
}

// groupKey provides prefixed key for a Group's marshalled data.
func groupKey(id types.GroupID) []byte {
	buf := bytes.NewBuffer(groupPrefix)
	buf.Write(id.Owner.Bytes())
	binary.Write(buf, binary.BigEndian, id.DSeq)
	binary.Write(buf, binary.BigEndian, id.GSeq)
	return buf.Bytes()
}

// groupOpenKey provides prefixed key for groups which are in open state.
// No data is stored under the key.
func groupOpenKey(id types.GroupID) []byte {
	buf := bytes.NewBuffer(groupOpenPrefix)
	buf.Write(id.Owner.Bytes())
	binary.Write(buf, binary.BigEndian, id.DSeq)
	binary.Write(buf, binary.BigEndian, id.GSeq)
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
	buf.Write(id.Owner.Bytes())
	binary.Write(buf, binary.BigEndian, id.DSeq)
	return buf.Bytes()
}

// groupsOpenKey provides store key for Groups in state open.
// Key is culled from the store once the Group is no longer in state Open.
// Uses the groupOpenPrefix byte.
func groupsOpenKey(id types.DeploymentID) []byte {
	buf := bytes.NewBuffer(groupOpenPrefix)
	buf.Write(id.Owner.Bytes())
	binary.Write(buf, binary.BigEndian, id.DSeq)
	return buf.Bytes()
}
