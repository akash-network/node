package keeper

import (
	"bytes"
	"encoding/binary"

	"github.com/ovrclk/akash/x/deployment/types"
)

var (
	deploymentPrefix = []byte{0x01}
	groupPrefix      = []byte{0x02}
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

// groupsKey provides default store Key for Group data.
func groupsKey(id types.DeploymentID) []byte {
	buf := bytes.NewBuffer(groupPrefix)
	buf.Write([]byte(id.Owner))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
