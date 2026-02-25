package keeper

import (
	"bytes"
	"encoding/binary"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"

	"pkg.akt.dev/go/node/deployment/v1"
	"pkg.akt.dev/go/node/deployment/v1beta4"
)

const (
	DeploymentStateActivePrefixID       = byte(0x01)
	DeploymentStateClosedPrefixID       = byte(0x02)
	GroupStateOpenPrefixID              = byte(0x01)
	GroupStatePausedPrefixID            = byte(0x02)
	GroupStateInsufficientFundsPrefixID = byte(0x03)
	GroupStateClosedPrefixID            = byte(0x04)
)

var (
	DeploymentPrefix                  = []byte{0x11, 0x00}
	DeploymentStateActivePrefix       = []byte{DeploymentStateActivePrefixID}
	DeploymentStateClosedPrefix       = []byte{DeploymentStateClosedPrefixID}
	GroupPrefix                       = []byte{0x12, 0x00}
	GroupStateOpenPrefix              = []byte{GroupStateOpenPrefixID}
	GroupStatePausedPrefix            = []byte{GroupStatePausedPrefixID}
	GroupStateInsufficientFundsPrefix = []byte{GroupStateInsufficientFundsPrefixID}
	GroupStateClosedPrefix            = []byte{GroupStateClosedPrefixID}
)

func DeploymentKey(statePrefix []byte, id v1.DeploymentID) ([]byte, error) {
	owner, err := sdk.AccAddressFromBech32(id.Owner)
	if err != nil {
		return nil, err
	}

	lenPrefixedOwner, err := address.LengthPrefix(owner)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(DeploymentPrefix)
	buf.Write(statePrefix)
	buf.Write(lenPrefixedOwner)

	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func MustDeploymentKey(statePrefix []byte, id v1.DeploymentID) []byte {
	key, err := DeploymentKey(statePrefix, id)
	if err != nil {
		panic(err)
	}
	return key
}

// GroupKey provides prefixed key for a Group's marshalled data.
func GroupKey(statePrefix []byte, id v1.GroupID) ([]byte, error) {
	owner, err := sdk.AccAddressFromBech32(id.Owner)
	if err != nil {
		return nil, err
	}

	lenPrefixedOwner, err := address.LengthPrefix(owner)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(GroupPrefix)
	buf.Write(statePrefix)

	buf.Write(lenPrefixedOwner)
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, id.GSeq); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func MustGroupKey(statePrefix []byte, id v1.GroupID) []byte {
	key, err := GroupKey(statePrefix, id)
	if err != nil {
		panic(err)
	}

	return key
}

func DeploymentStateToPrefix(state v1.Deployment_State) []byte {
	var idx []byte

	switch state {
	case v1.DeploymentActive:
		idx = DeploymentStateActivePrefix
	case v1.DeploymentClosed:
		idx = DeploymentStateClosedPrefix
	}

	return idx
}

func GroupStateToPrefix(state v1beta4.Group_State) []byte {
	var idx []byte
	switch state {
	case v1beta4.GroupOpen:
		idx = GroupStateOpenPrefix
	case v1beta4.GroupPaused:
		idx = GroupStatePausedPrefix
	case v1beta4.GroupInsufficientFunds:
		idx = GroupStateInsufficientFundsPrefix
	case v1beta4.GroupClosed:
		idx = GroupStateClosedPrefix
	}

	return idx
}
