package keeper

import (
	"bytes"
	"encoding/binary"

	"github.com/cosmos/cosmos-sdk/types/address"

	"github.com/ovrclk/akash/sdkutil"

	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/ovrclk/akash/x/market/types"
)

var (
	orderPrefix = []byte{0x01, 0x00}
	bidPrefix   = []byte{0x02, 0x00}
	leasePrefix = []byte{0x03, 0x00}
)

func filterToPrefix(prefix []byte, owner string, dseq uint64, gseq, oseq uint32, provider string) ([]byte, error) {
	buf := bytes.NewBuffer(prefix)

	if len(owner) == 0 {
		return buf.Bytes(), nil
	}
	if _, err := buf.Write([]byte(owner)); err != nil {
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
	if _, err := buf.Write([]byte(provider)); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func orderPrefixFromFilter(f types.OrderFilters) ([]byte, error) {
	return filterToPrefix(orderPrefix, f.Owner, f.DSeq, f.GSeq, f.OSeq, "")
}

func leasePrefixFromFilter(f types.LeaseFilters) ([]byte, error) {
	return filterToPrefix(leasePrefix, f.Owner, f.DSeq, f.GSeq, f.OSeq, f.Provider)
}

func bidPrefixFromFilter(f types.BidFilters) ([]byte, error) {
	return filterToPrefix(bidPrefix, f.Owner, f.DSeq, f.GSeq, f.OSeq, f.Provider)
}

func orderKey(id types.OrderID) []byte {
	buf := bytes.NewBuffer(orderPrefix)
	buf.Write([]byte(id.Owner))
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

func bidKey(id types.BidID) []byte {
	buf := bytes.NewBuffer(bidPrefix)
	buf.Write([]byte(id.Owner))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	if err := binary.Write(buf, binary.BigEndian, id.GSeq); err != nil {
		panic(err)
	}
	if err := binary.Write(buf, binary.BigEndian, id.OSeq); err != nil {
		panic(err)
	}
	buf.Write([]byte(id.Provider))
	return buf.Bytes()
}

func leaseKey(id types.LeaseID) []byte {
	buf := bytes.NewBuffer(leasePrefix)
	buf.Write([]byte(id.Owner))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	if err := binary.Write(buf, binary.BigEndian, id.GSeq); err != nil {
		panic(err)
	}
	if err := binary.Write(buf, binary.BigEndian, id.OSeq); err != nil {
		panic(err)
	}
	buf.Write([]byte(id.Provider))
	return buf.Bytes()
}

func ordersForGroupPrefix(id dtypes.GroupID) []byte {
	buf := bytes.NewBuffer(orderPrefix)
	buf.Write([]byte(id.Owner))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	if err := binary.Write(buf, binary.BigEndian, id.GSeq); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func bidsForOrderPrefix(id types.OrderID) []byte {
	buf := bytes.NewBuffer(bidPrefix)
	buf.Write([]byte(id.Owner))
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
