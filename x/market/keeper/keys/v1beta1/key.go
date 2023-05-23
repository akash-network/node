package v1beta1

import (
	"bytes"
	"encoding/binary"

	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta1"
	types "github.com/akash-network/akash-api/go/node/market/v1beta1"
)

var (
	orderPrefix = []byte{0x01, 0x00}
	bidPrefix   = []byte{0x02, 0x00}
	leasePrefix = []byte{0x03, 0x00} //nolint:unused // this will be used in the future
)

func orderKey(id types.OrderID) []byte { //nolint:unused // this will be used in the future
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

func bidKey(id types.BidID) []byte { //nolint:unused // this will be used in the future
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

func leaseKey(id types.LeaseID) []byte { //nolint:unused // this will be used in the future
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

func OrdersForGroupPrefix(id dtypes.GroupID) []byte {
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

func BidsForOrderPrefix(id types.OrderID) []byte {
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
