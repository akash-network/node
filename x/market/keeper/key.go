package keeper

import (
	"bytes"
	"encoding/binary"

	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/ovrclk/akash/x/market/types"
)

var (
	orderPrefix       = []byte{0x01, 0x00}
	bidPrefix         = []byte{0x02, 0x00}
	leasePrefix       = []byte{0x03, 0x00}
	leaseActivePrefix = []byte{0x03, 0x01}
)

func orderKey(id types.OrderID) []byte {
	buf := bytes.NewBuffer(orderPrefix)
	buf.Write(id.Owner.Bytes())
	binary.Write(buf, binary.BigEndian, id.DSeq)
	binary.Write(buf, binary.BigEndian, id.GSeq)
	binary.Write(buf, binary.BigEndian, id.OSeq)
	return buf.Bytes()
}

func bidKey(id types.BidID) []byte {
	buf := bytes.NewBuffer(bidPrefix)
	buf.Write(id.Owner.Bytes())
	binary.Write(buf, binary.BigEndian, id.DSeq)
	binary.Write(buf, binary.BigEndian, id.GSeq)
	binary.Write(buf, binary.BigEndian, id.OSeq)
	buf.Write(id.Provider.Bytes())
	return buf.Bytes()
}

func leaseKey(id types.LeaseID) []byte {
	buf := bytes.NewBuffer(leasePrefix)
	buf.Write(id.Owner.Bytes())
	binary.Write(buf, binary.BigEndian, id.DSeq)
	binary.Write(buf, binary.BigEndian, id.GSeq)
	binary.Write(buf, binary.BigEndian, id.OSeq)
	buf.Write(id.Provider.Bytes())
	return buf.Bytes()
}

func leaseKeyActive(id types.LeaseID) []byte {
	buf := bytes.NewBuffer(leaseActivePrefix)
	buf.Write(id.Owner.Bytes())
	binary.Write(buf, binary.BigEndian, id.DSeq)
	binary.Write(buf, binary.BigEndian, id.GSeq)
	binary.Write(buf, binary.BigEndian, id.OSeq)
	buf.Write(id.Provider.Bytes())
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
	buf.Write(id.Owner.Bytes())
	binary.Write(buf, binary.BigEndian, id.DSeq)
	binary.Write(buf, binary.BigEndian, id.GSeq)
	return buf.Bytes()
}

func bidsForOrderPrefix(id types.OrderID) []byte {
	buf := bytes.NewBuffer(bidPrefix)
	buf.Write(id.Owner.Bytes())
	binary.Write(buf, binary.BigEndian, id.DSeq)
	binary.Write(buf, binary.BigEndian, id.GSeq)
	binary.Write(buf, binary.BigEndian, id.OSeq)
	return buf.Bytes()
}
