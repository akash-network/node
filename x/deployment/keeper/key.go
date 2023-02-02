package keeper

import (
	"bytes"
	"encoding/binary"

	"github.com/cosmos/cosmos-sdk/types/address"

	types "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	"github.com/akash-network/akash-api/go/sdkutil"
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

func filterToPrefix(prefix []byte, owner string, dseq uint64, gseq uint32) ([]byte, error) {
	buf := bytes.NewBuffer(prefix)

	if len(owner) == 0 {
		return buf.Bytes(), nil
	}

	if _, err := buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(owner))); err != nil {
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
	return filterToPrefix(types.DeploymentPrefix(), f.Owner, f.DSeq, 0)
}
