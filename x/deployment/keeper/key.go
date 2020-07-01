package keeper

import (
	"bytes"
	"encoding/binary"

	"github.com/ovrclk/akash/x/deployment/types"
)

var (
	deploymentPrefixBase = []byte{0x01}
	groupPrefix          = []byte{0x02}

	possibleDeploymentStates = []types.DeploymentState{
		types.DeploymentActive,
		types.DeploymentClosed,
	}
)

// deploymentStatelessIDKeys provides all possible keys in expected short-circuiting
// order for querying a Deployment with only the ID.
//  0x01[State][OwnerID][DSeq]
func deploymentStatelessIDKeys(id types.DeploymentID) ([][]byte, error) {
	out := make([][]byte, 0, len(possibleDeploymentStates))
	// Prioritize "Active" state first as it will be the most common, then closed.
	for _, v := range possibleDeploymentStates {
		prefix, err := deploymentStateKey(v)
		if err != nil {
			return nil, err
		}
		buf := bytes.NewBuffer(prefix)
		_, err = buf.Write(id.Owner.Bytes())
		if err != nil {
			return nil, err
		}
		err = binary.Write(buf, binary.BigEndian, id.DSeq)
		if err != nil {
			return nil, err
		}

		out = append(out, buf.Bytes())
	}
	return out, nil
}

// deploymentStateKey returns 0x01[State] component of the Deployment index.
func deploymentStateKey(d types.DeploymentState) ([]byte, error) {
	buf := bytes.NewBuffer(deploymentPrefixBase)
	var err error
	err = binary.Write(buf, binary.BigEndian, d)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// deploymentStateIDKey returns 0x01[State][OwnerID][DSeq] complete
// Deployment index value.
func deploymentStateIDKey(d types.Deployment) ([]byte, error) {
	b, err := deploymentStateKey(d.State)
	if err != nil {
		return nil, err
	}
	buf := bytes.NewBuffer(b)

	_, err = buf.Write(d.ID().Owner.Bytes())
	if err != nil {
		return nil, err
	}
	err = binary.Write(buf, binary.BigEndian, d.DSeq)
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
