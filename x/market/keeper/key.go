package keeper

import (
	"bytes"
	"encoding/binary"

	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/ovrclk/akash/x/market/types"
)

var (
	orderPrefix       = []byte{0x01, 0x00}
	orderOpenPrefix   = []byte{0x01, 0x01}
	bidPrefix         = []byte{0x02, 0x00}
	leasePrefix       = []byte{0x03, 0x00}
	leaseActivePrefix = []byte{0x03, 0x01}
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

func orderOpenKey(id types.OrderID) []byte {
	buf := bytes.NewBuffer(orderOpenPrefix)
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

func convertOrderOpenKey(activeKey []byte) ([]byte, error) {
	buf := bytes.NewBuffer(orderPrefix)
	_, err := buf.Write(activeKey[len(orderOpenPrefix):])
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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

func leaseKeyActive(id types.LeaseID) []byte {
	buf := bytes.NewBuffer(leaseActivePrefix)
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

func convertLeaseActiveKey(activeKey []byte) ([]byte, error) {
	buf := bytes.NewBuffer(leasePrefix)
	_, err := buf.Write(activeKey[len(leaseActivePrefix):])
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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
