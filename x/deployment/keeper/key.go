package keeper

import (
	"bytes"
	"encoding/binary"

	"github.com/ovrclk/akash/sdkutil"

	"github.com/cosmos/cosmos-sdk/types/address"

	types "github.com/ovrclk/akash/x/deployment/types/v1beta2"
)

func deploymentKey(id types.DeploymentID) []byte {
	buf := bytes.NewBuffer(types.DeploymentPrefix())
	buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(id.Owner)))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// groupKey provides prefixed key for a Group's marshalled data.
func groupKey(id types.GroupID) []byte {
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

// groupsKey provides default store Key for Group data.
func groupsKey(id types.DeploymentID) []byte {
	buf := bytes.NewBuffer(types.GroupPrefix())
	buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(id.Owner)))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
