package keeper

import (
	"bytes"
	"encoding/binary"

	"github.com/cosmos/cosmos-sdk/types/address"

	"pkg.akt.dev/go/node/deployment/v1"
	"pkg.akt.dev/go/node/deployment/v1beta4"
	"pkg.akt.dev/go/sdkutil"
)

func deploymentKey(id v1.DeploymentID) []byte {
	buf := bytes.NewBuffer(v1.DeploymentPrefix())
	buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(id.Owner)))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// groupKey provides prefixed key for a Group's marshalled data.
func groupKey(id v1.GroupID) []byte {
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

// groupsKey provides default store Key for Group data.
func groupsKey(id v1.DeploymentID) []byte {
	buf := bytes.NewBuffer(v1.GroupPrefix())
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

func deploymentPrefixFromFilter(f v1beta4.DeploymentFilters) ([]byte, error) {
	return filterToPrefix(v1.DeploymentPrefix(), f.Owner, f.DSeq, 0)
}
