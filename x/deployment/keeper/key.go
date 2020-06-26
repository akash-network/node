package keeper

import (
	"bytes"
	"encoding/binary"

	"github.com/ovrclk/akash/x/deployment/types"
)

var (
	deploymentPrefix = []byte{0x01, 0x00}
	groupPrefix      = []byte{0x02}

	deploymentActivePrefix = []byte{0x01, 0x01}
	deploymentClosedPrefix = []byte{0x01, 0x02}
)

func deploymentIDKey(id types.DeploymentID) []byte {
	buf := bytes.NewBuffer(deploymentPrefix)
	buf.Write(id.Owner.Bytes())
	binary.Write(buf, binary.BigEndian, id.DSeq)
	return buf.Bytes()
}

func deploymentKey(d types.Deployment) ([]byte, error) {
	var buf *bytes.Buffer
	var err error

	switch d.State {
	case types.DeploymentActive:
		_, err = buf.Write(deploymentActivePrefix)
	case types.DeploymentClosed:
		_, err = buf.Write(deploymentClosedPrefix)
	}
	if err != nil {
		return nil, err
	}

	err = binary.Write(buf, binary.BigEndian, d.ID().DSeq)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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
