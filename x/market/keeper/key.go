package keeper

import (
	"bytes"
	"encoding/binary"

	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/ovrclk/akash/x/market/types"
)

var (
	orderPrefix = []byte{0x01, 0x00}
	bidPrefix   = []byte{0x02, 0x00}
	leasePrefix = []byte{0x03, 0x00}
)

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
