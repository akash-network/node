package keeper

import (
	"bytes"
	"encoding/binary"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"

	"pkg.akt.dev/go/node/deployment/v1"
	"pkg.akt.dev/go/node/deployment/v1beta4"
	"pkg.akt.dev/go/sdkutil"
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

// GroupsKey provides default store Key for Group data.
func GroupsKey(statePrefix []byte, id v1.DeploymentID) ([]byte, error) {
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

func MustGroupsKey(statePrefix []byte, id v1.DeploymentID) []byte {
	key, err := GroupsKey(statePrefix, id)
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

func buildDeploymentPrefix(state v1.Deployment_State) []byte {
	idx := DeploymentStateToPrefix(state)

	res := make([]byte, 0, len(DeploymentPrefix)+len(idx))
	res = append(res, DeploymentPrefix...)
	res = append(res, idx...)

	return res
}

// nolint: unused
func buildGroupPrefix(state v1beta4.Group_State) []byte {
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

func deploymentPrefixFromFilter(f v1beta4.DeploymentFilters) ([]byte, error) {
	return filterToPrefix(buildDeploymentPrefix(v1.Deployment_State(v1.Deployment_State_value[f.State])), f.Owner, f.DSeq, 0)
}

func DeploymentKeyLegacy(id v1.DeploymentID) []byte {
	buf := bytes.NewBuffer(v1.DeploymentPrefix())
	buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(id.Owner)))

	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}

	return buf.Bytes()
}

// GroupKeyLegacy provides prefixed key for a Group's marshalled data.
func GroupKeyLegacy(id v1.GroupID) []byte {
	buf := bytes.NewBuffer(v1.GroupPrefix())
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
func GroupsKeyLegacy(id v1.DeploymentID) []byte {
	buf := bytes.NewBuffer(v1.GroupPrefix())
	buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(id.Owner)))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// nolint: unused
func deploymentPrefixFromFilterLegacy(f v1beta4.DeploymentFilters) ([]byte, error) {
	return filterToPrefix(v1.DeploymentPrefix(), f.Owner, f.DSeq, 0)
}
