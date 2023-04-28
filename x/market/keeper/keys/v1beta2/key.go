package v1beta2

import (
	"bytes"
	"encoding/binary"

	"github.com/cosmos/cosmos-sdk/types/address"

	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta2"
	types "github.com/akash-network/akash-api/go/node/market/v1beta2"
	"github.com/akash-network/akash-api/go/sdkutil"
)

func filterToPrefix(prefix []byte, owner string, dseq uint64, gseq, oseq uint32, provider string) ([]byte, error) {
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

	if oseq == 0 {
		return buf.Bytes(), nil
	}
	if err := binary.Write(buf, binary.BigEndian, oseq); err != nil {
		return nil, err
	}

	if len(provider) == 0 {
		return buf.Bytes(), nil
	}

	if _, err := buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(provider))); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func OrderPrefixFromFilter(f types.OrderFilters) ([]byte, error) {
	return filterToPrefix(types.OrderPrefix(), f.Owner, f.DSeq, f.GSeq, f.OSeq, "")
}

func LeasePrefixFromFilter(f types.LeaseFilters) ([]byte, bool, error) {
	prefix, err := filterToPrefix(types.LeasePrefix(), f.Owner, f.DSeq, f.GSeq, f.OSeq, f.Provider)
	return prefix, false, err
}

func BidPrefixFromFilter(f types.BidFilters) ([]byte, error) {
	return filterToPrefix(types.BidPrefix(), f.Owner, f.DSeq, f.GSeq, f.OSeq, f.Provider)
}

func OrderKey(id types.OrderID) []byte {
	buf := bytes.NewBuffer(types.OrderPrefix())
	buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(id.Owner)))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	if err := binary.Write(buf, binary.BigEndian, id.GSeq); err != nil {
		panic(err)
	}
	if err := binary.Write(buf, binary.BigEndian, id.OSeq); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func BidKey(id types.BidID) []byte {
	buf := bytes.NewBuffer(types.BidPrefix())
	buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(id.Owner)))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	if err := binary.Write(buf, binary.BigEndian, id.GSeq); err != nil {
		panic(err)
	}
	if err := binary.Write(buf, binary.BigEndian, id.OSeq); err != nil {
		panic(err)
	}
	buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(id.Provider)))
	return buf.Bytes()
}

func LeaseKey(id types.LeaseID) []byte {
	buf := bytes.NewBuffer(types.LeasePrefix())
	buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(id.Owner)))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	if err := binary.Write(buf, binary.BigEndian, id.GSeq); err != nil {
		panic(err)
	}
	if err := binary.Write(buf, binary.BigEndian, id.OSeq); err != nil {
		panic(err)
	}
	buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(id.Provider)))
	return buf.Bytes()
}

func secondaryLeaseKeyByProvider(id types.LeaseID) []byte {
	buf := bytes.NewBuffer(types.SecondaryLeasePrefix())
	buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(id.Provider)))
	buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(id.Owner)))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	if err := binary.Write(buf, binary.BigEndian, id.GSeq); err != nil {
		panic(err)
	}
	if err := binary.Write(buf, binary.BigEndian, id.OSeq); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func SecondaryKeysForLease(id types.LeaseID) [][]byte {
	return [][]byte{
		secondaryLeaseKeyByProvider(id),
	}
}

func OrdersForGroupPrefix(id dtypes.GroupID) []byte {
	buf := bytes.NewBuffer(types.OrderPrefix())
	buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(id.Owner)))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	if err := binary.Write(buf, binary.BigEndian, id.GSeq); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func BidsForOrderPrefix(id types.OrderID) []byte {
	buf := bytes.NewBuffer(types.BidPrefix())
	buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(id.Owner)))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	if err := binary.Write(buf, binary.BigEndian, id.GSeq); err != nil {
		panic(err)
	}
	if err := binary.Write(buf, binary.BigEndian, id.OSeq); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
