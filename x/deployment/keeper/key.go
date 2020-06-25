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
	buf.Write(id.Owner.Bytes())
	binary.Write(buf, binary.BigEndian, id.DSeq)
	return buf.Bytes()
}

func deploymentStateKey(d types.Deployment) []byte {
	buf := bytes.NewBuffer(deploymentPrefix)
	binary.Write(buf, binary.BigEndian, d.State)
	return buf.Bytes()
}

func groupKey(id types.GroupID) []byte {
	buf := bytes.NewBuffer(groupPrefix)
	buf.Write(id.Owner.Bytes())
	binary.Write(buf, binary.BigEndian, id.DSeq)
	binary.Write(buf, binary.BigEndian, id.GSeq)
	return buf.Bytes()
}

func groupsKey(id types.DeploymentID) []byte {
	buf := bytes.NewBuffer(groupPrefix)
	buf.Write(id.Owner.Bytes())
	binary.Write(buf, binary.BigEndian, id.DSeq)
	return buf.Bytes()
}
