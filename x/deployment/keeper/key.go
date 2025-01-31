package keeper

import (
	"bytes"
	"encoding/binary"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"

	types "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	"github.com/akash-network/akash-api/go/sdkutil"
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

func DeploymentKey(statePrefix []byte, id types.DeploymentID) ([]byte, error) {
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

func MustDeploymentKey(statePrefix []byte, id types.DeploymentID) []byte {
	key, err := DeploymentKey(statePrefix, id)
	if err != nil {
		panic(err)
	}
	return key
}

// GroupKey provides prefixed key for a Group's marshalled data.
func GroupKey(statePrefix []byte, id types.GroupID) ([]byte, error) {
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

func MustGroupKey(statePrefix []byte, id types.GroupID) []byte {
	key, err := GroupKey(statePrefix, id)
	if err != nil {
		panic(err)
	}
	return key
}

// GroupsKey provides default store Key for Group data.
func GroupsKey(statePrefix []byte, id types.DeploymentID) ([]byte, error) {
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

	return buf.Bytes(), nil
}

func MustGroupsKey(statePrefix []byte, id types.DeploymentID) []byte {
	key, err := GroupsKey(statePrefix, id)
	if err != nil {
		panic(err)
	}
	return key
}

func DeploymentStateToPrefix(state types.Deployment_State) []byte {
	var idx []byte

	switch state {
	case types.DeploymentActive:
		idx = DeploymentStateActivePrefix
	case types.DeploymentClosed:
		idx = DeploymentStateClosedPrefix
	}

	return idx
}

func GroupStateToPrefix(state types.Group_State) []byte {
	var idx []byte
	switch state {
	case types.GroupOpen:
		idx = GroupStateOpenPrefix
	case types.GroupPaused:
		idx = GroupStatePausedPrefix
	case types.GroupInsufficientFunds:
		idx = GroupStateInsufficientFundsPrefix
	case types.GroupClosed:
		idx = GroupStateClosedPrefix
	}

	return idx
}

func buildDeploymentPrefix(state types.Deployment_State) []byte {
	idx := DeploymentStateToPrefix(state)

	res := make([]byte, 0, len(DeploymentPrefix)+len(idx))
	res = append(res, DeploymentPrefix...)
	res = append(res, idx...)

	return res
}

// nolint: unused
func buildGroupPrefix(state types.Group_State) []byte {
	idx := GroupStateToPrefix(state)

	res := make([]byte, 0, len(GroupPrefix)+len(idx))
	res = append(res, GroupPrefix...)
	res = append(res, idx...)

	return res
}

func filterToPrefix(prefix []byte, owner string, dseq uint64, gseq uint32) ([]byte, error) {
	buf := bytes.NewBuffer(prefix)

	if len(owner) == 0 {
		return buf.Bytes(), nil
	}

	ownerAddr, err := sdk.AccAddressFromBech32(owner)
	if err != nil {
		return nil, err
	}

	lenPrefixedOwner, err := address.LengthPrefix(ownerAddr)
	if err != nil {
		return nil, err
	}

	if _, err := buf.Write(lenPrefixedOwner); err != nil {
		return nil, err
	}

	if dseq == 0 {
		return buf.Bytes(), nil
	}

	if err := binary.Write(buf, binary.BigEndian, dseq); err != nil {
		return nil, err
	}

	if gseq == 0 {
		return buf.Bytes(), nil
	}

	if err := binary.Write(buf, binary.BigEndian, gseq); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func deploymentPrefixFromFilter(f types.DeploymentFilters) ([]byte, error) {
	return filterToPrefix(buildDeploymentPrefix(types.Deployment_State(types.Deployment_State_value[f.State])), f.Owner, f.DSeq, 0)
}

func DeploymentKeyLegacy(id types.DeploymentID) []byte {
	buf := bytes.NewBuffer(types.DeploymentPrefix())
	buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(id.Owner)))

	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}

	return buf.Bytes()
}

// GroupKeyLegacy provides prefixed key for a Group's marshalled data.
func GroupKeyLegacy(id types.GroupID) []byte {
	buf := bytes.NewBuffer(types.GroupPrefix())
	buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(id.Owner)))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	if err := binary.Write(buf, binary.BigEndian, id.GSeq); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// GroupsKeyLegacy provides default store Key for Group data.
func GroupsKeyLegacy(id types.DeploymentID) []byte {
	buf := bytes.NewBuffer(types.GroupPrefix())
	buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(id.Owner)))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// nolint: unused
func deploymentPrefixFromFilterLegacy(f types.DeploymentFilters) ([]byte, error) {
	return filterToPrefix(types.DeploymentPrefix(), f.Owner, f.DSeq, 0)
}
