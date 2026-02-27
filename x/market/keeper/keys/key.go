package keys

import (
	"bytes"
	"encoding/binary"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"

	dtypes "pkg.akt.dev/go/node/deployment/v1"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"
	"pkg.akt.dev/go/sdkutil"
)

const (
	OrderStateOpenPrefixID              = byte(0x01)
	OrderStateActivePrefixID            = byte(0x02)
	OrderStateClosedPrefixID            = byte(0x03)
	BidStateOpenPrefixID                = byte(0x01)
	BidStateActivePrefixID              = byte(0x02)
	BidStateLostPrefixID                = byte(0x03)
	BidStateClosedPrefixID              = byte(0x04)
	LeaseStateActivePrefixID            = byte(0x01)
	LeaseStateInsufficientFundsPrefixID = byte(0x02)
	LeaseStateClosedPrefixID            = byte(0x03)
)

var (
	OrderPrefix                       = []byte{0x11, 0x00}
	OrderPrefixNew                    = []byte{0x11, 0x01}
	OrderIndexStatePrefix             = []byte{0x11, 0x02}
	OrderIndexGroupStatePrefix        = []byte{0x11, 0x03}
	OrderStateOpenPrefix              = []byte{OrderStateOpenPrefixID}
	OrderStateActivePrefix            = []byte{OrderStateActivePrefixID}
	OrderStateClosedPrefix            = []byte{OrderStateClosedPrefixID}
	BidPrefix                         = []byte{0x12, 0x00}
	BidPrefixReverse                  = []byte{0x12, 0x01}
	BidPrefixNew                      = []byte{0x12, 0x02}
	BidIndexStatePrefix               = []byte{0x12, 0x03}
	BidIndexProviderPrefix            = []byte{0x12, 0x04}
	BidIndexOrderStatePrefix          = []byte{0x12, 0x05}
	BidStateOpenPrefix                = []byte{BidStateOpenPrefixID}
	BidStateActivePrefix              = []byte{BidStateActivePrefixID}
	BidStateLostPrefix                = []byte{BidStateLostPrefixID}
	BidStateClosedPrefix              = []byte{BidStateClosedPrefixID}
	LeasePrefix                       = []byte{0x13, 0x00}
	LeasePrefixReverse                = []byte{0x13, 0x01}
	LeasePrefixNew                    = []byte{0x13, 0x02}
	LeaseIndexStatePrefix             = []byte{0x13, 0x03}
	LeaseIndexProviderPrefix          = []byte{0x13, 0x04}
	LeaseStateActivePrefix            = []byte{LeaseStateActivePrefixID}
	LeaseStateInsufficientFundsPrefix = []byte{LeaseStateInsufficientFundsPrefixID}
	LeaseStateClosedPrefix            = []byte{LeaseStateClosedPrefixID}
)

func OrderKey(statePrefix []byte, id mv1.OrderID) ([]byte, error) {
	owner, err := sdk.AccAddressFromBech32(id.Owner)
	if err != nil {
		return nil, err
	}

	lenPrefixedOwner, err := address.LengthPrefix(owner)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(OrderPrefix)
	buf.Write(statePrefix)
	buf.Write(lenPrefixedOwner)

	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, id.GSeq); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, id.OSeq); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func MustOrderKey(statePrefix []byte, id mv1.OrderID) []byte {
	key, err := OrderKey(statePrefix, id)
	if err != nil {
		panic(err)
	}
	return key
}

func BidKey(statePrefix []byte, id mv1.BidID) ([]byte, error) {
	owner, err := sdk.AccAddressFromBech32(id.Owner)
	if err != nil {
		return nil, err
	}

	lenPrefixedOwner, err := address.LengthPrefix(owner)
	if err != nil {
		return nil, err
	}

	provider, err := sdk.AccAddressFromBech32(id.Provider)
	if err != nil {
		return nil, err
	}

	lenPrefixedProvider, err := address.LengthPrefix(provider)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(BidPrefix)
	buf.Write(statePrefix)

	buf.Write(lenPrefixedOwner)
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, id.GSeq); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, id.OSeq); err != nil {
		return nil, err
	}

	buf.Write(lenPrefixedProvider)

	if err := binary.Write(buf, binary.BigEndian, id.BSeq); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func MustBidKey(statePrefix []byte, id mv1.BidID) []byte {
	key, err := BidKey(statePrefix, id)
	if err != nil {
		panic(err)
	}
	return key
}

func BidReverseKey(statePrefix []byte, id mv1.BidID) ([]byte, error) {
	owner, err := sdk.AccAddressFromBech32(id.Owner)
	if err != nil {
		return nil, err
	}

	lenPrefixedOwner, err := address.LengthPrefix(owner)
	if err != nil {
		return nil, err
	}

	provider, err := sdk.AccAddressFromBech32(id.Provider)
	if err != nil {
		return nil, err
	}

	lenPrefixedProvider, err := address.LengthPrefix(provider)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(BidPrefixReverse)

	buf.Write(statePrefix)
	buf.Write(lenPrefixedProvider)

	if err := binary.Write(buf, binary.BigEndian, id.BSeq); err != nil {
		return nil, err
	}

	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, id.GSeq); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, id.OSeq); err != nil {
		return nil, err
	}

	buf.Write(lenPrefixedOwner)

	return buf.Bytes(), nil
}

func MustBidReverseKey(statePrefix []byte, id mv1.BidID) []byte {
	key, err := BidReverseKey(statePrefix, id)
	if err != nil {
		panic(err)
	}
	return key
}

func BidStateReverseKey(state mtypes.Bid_State, id mv1.BidID) ([]byte, error) {
	if state != mtypes.BidActive && state != mtypes.BidOpen {
		return nil, nil
	}

	prefix := BidStateToPrefix(state)
	key, err := BidReverseKey(prefix, id)
	if err != nil {
		return nil, err
	}

	return key, nil
}

func MustBidStateRevereKey(state mtypes.Bid_State, id mv1.BidID) []byte {
	key, err := BidStateReverseKey(state, id)
	if err != nil {
		panic(err)
	}

	return key
}

func LeaseKey(statePrefix []byte, id mv1.LeaseID) ([]byte, error) {
	owner, err := sdk.AccAddressFromBech32(id.Owner)
	if err != nil {
		return nil, err
	}

	lenPrefixedOwner, err := address.LengthPrefix(owner)
	if err != nil {
		return nil, err
	}

	provider, err := sdk.AccAddressFromBech32(id.Provider)
	if err != nil {
		return nil, err
	}

	lenPrefixedProvider, err := address.LengthPrefix(provider)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(LeasePrefix)
	buf.Write(statePrefix)
	buf.Write(lenPrefixedOwner)

	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, id.GSeq); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, id.OSeq); err != nil {
		return nil, err
	}

	buf.Write(lenPrefixedProvider)

	if err := binary.Write(buf, binary.BigEndian, id.BSeq); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func MustLeaseKey(statePrefix []byte, id mv1.LeaseID) []byte {
	key, err := LeaseKey(statePrefix, id)
	if err != nil {
		panic(err)
	}
	return key
}

func LeaseReverseKey(statePrefix []byte, id mv1.LeaseID) ([]byte, error) {
	owner, err := sdk.AccAddressFromBech32(id.Owner)
	if err != nil {
		return nil, err
	}

	lenPrefixedOwner, err := address.LengthPrefix(owner)
	if err != nil {
		return nil, err
	}

	provider, err := sdk.AccAddressFromBech32(id.Provider)
	if err != nil {
		return nil, err
	}

	lenPrefixedProvider, err := address.LengthPrefix(provider)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(LeasePrefixReverse)
	buf.Write(statePrefix)
	buf.Write(lenPrefixedProvider)
	if err := binary.Write(buf, binary.BigEndian, id.BSeq); err != nil {
		return nil, err
	}

	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, id.GSeq); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, id.OSeq); err != nil {
		return nil, err
	}

	buf.Write(lenPrefixedOwner)

	return buf.Bytes(), nil
}

func LeaseStateReverseKey(state mv1.Lease_State, id mv1.LeaseID) ([]byte, error) {
	if state != mv1.LeaseActive {
		return nil, nil
	}

	prefix := LeaseStateToPrefix(state)
	key, err := LeaseReverseKey(prefix, id)
	if err != nil {
		return nil, err
	}

	return key, nil
}

func MustLeaseStateReverseKey(state mv1.Lease_State, id mv1.LeaseID) []byte {
	key, err := LeaseStateReverseKey(state, id)
	if err != nil {
		panic(err)
	}

	return key
}

func MustLeaseReverseKey(statePrefix []byte, id mv1.LeaseID) []byte {
	key, err := LeaseReverseKey(statePrefix, id)
	if err != nil {
		panic(err)
	}
	return key
}

func OrdersForGroupPrefix(statePrefix []byte, id dtypes.GroupID) []byte {
	buf := bytes.NewBuffer(OrderPrefix)
	buf.Write(statePrefix)
	buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(id.Owner)))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	if err := binary.Write(buf, binary.BigEndian, id.GSeq); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func BidsForOrderPrefix(statePrefix []byte, id mv1.OrderID) []byte {
	buf := bytes.NewBuffer(BidPrefix)
	buf.Write(statePrefix)
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

func OrderStateToPrefix(state mtypes.Order_State) []byte {
	var res []byte

	switch state {
	case mtypes.OrderOpen:
		res = OrderStateOpenPrefix
	case mtypes.OrderActive:
		res = OrderStateActivePrefix
	case mtypes.OrderClosed:
		res = OrderStateClosedPrefix
	}

	return res
}

func BidStateToPrefix(state mtypes.Bid_State) []byte {
	var res []byte

	switch state {
	case mtypes.BidOpen:
		res = BidStateOpenPrefix
	case mtypes.BidActive:
		res = BidStateActivePrefix
	case mtypes.BidLost:
		res = BidStateLostPrefix
	case mtypes.BidClosed:
		res = BidStateClosedPrefix
	}

	return res
}

func LeaseStateToPrefix(state mv1.Lease_State) []byte {
	var res []byte

	switch state {
	case mv1.LeaseActive:
		res = LeaseStateActivePrefix
	case mv1.LeaseInsufficientFunds:
		res = LeaseStateInsufficientFundsPrefix
	case mv1.LeaseClosed:
		res = LeaseStateClosedPrefix
	}

	return res
}

func filterToPrefix(prefix []byte, owner string, dseq uint64, gseq, oseq uint32, provider string, bseq uint32) ([]byte, error) {
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

	if bseq == 0 {
		return buf.Bytes(), nil
	}
	if err := binary.Write(buf, binary.BigEndian, bseq); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// nolint: unused
func reverseFilterToPrefix(prefix []byte, provider string, bseq uint32, dseq uint64, gseq, oseq uint32, owner string) ([]byte, error) {
	buf := bytes.NewBuffer(prefix)

	if len(provider) == 0 {
		return buf.Bytes(), nil
	}

	if _, err := buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(provider))); err != nil {
		return nil, err
	}

	if bseq == 0 {
		return buf.Bytes(), nil
	}
	if err := binary.Write(buf, binary.BigEndian, bseq); err != nil {
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

	if len(owner) == 0 {
		return buf.Bytes(), nil
	}

	if _, err := buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(owner))); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func OrderPrefixFromFilter(f mtypes.OrderFilters) ([]byte, error) {
	var idx []byte
	switch f.State {
	case mtypes.OrderOpen.String():
		idx = OrderStateOpenPrefix
	case mtypes.OrderActive.String():
		idx = OrderStateActivePrefix
	case mtypes.OrderClosed.String():
		idx = OrderStateClosedPrefix
	}

	prefix := make([]byte, 0, len(OrderPrefix)+len(idx))
	prefix = append(prefix, OrderPrefix...)
	prefix = append(prefix, idx...)

	return filterToPrefix(prefix, f.Owner, f.DSeq, f.GSeq, f.OSeq, "", 0)
}

func buildLeasePrefix(prefix []byte, state string) []byte {
	var idx []byte
	switch state {
	case mv1.LeaseActive.String():
		idx = LeaseStateActivePrefix
	case mv1.LeaseInsufficientFunds.String():
		idx = LeaseStateInsufficientFundsPrefix
	case mv1.LeaseClosed.String():
		idx = LeaseStateClosedPrefix
	}

	res := make([]byte, 0, len(prefix)+len(idx))
	res = append(res, prefix...)
	res = append(res, idx...)

	return res
}

func buildBidPrefix(prefix []byte, state string) []byte {
	var idx []byte
	switch state {
	case mtypes.BidActive.String():
		idx = BidStateActivePrefix
	case mtypes.BidOpen.String():
		idx = BidStateOpenPrefix
	case mtypes.BidLost.String():
		idx = BidStateLostPrefix
	case mtypes.BidClosed.String():
		idx = BidStateClosedPrefix
	}

	res := make([]byte, 0, len(prefix)+len(idx))
	res = append(res, prefix...)
	res = append(res, idx...)

	return res
}

func BidPrefixFromFilter(f mtypes.BidFilters) ([]byte, error) {
	return filterToPrefix(buildBidPrefix(BidPrefix, f.State), f.Owner, f.DSeq, f.GSeq, f.OSeq, f.Provider, f.BSeq)
}

func BidReversePrefixFromFilter(f mtypes.BidFilters) ([]byte, error) {
	prefix, err := reverseFilterToPrefix(buildBidPrefix(BidPrefixReverse, f.State), f.Provider, f.BSeq, f.DSeq, f.GSeq, f.OSeq, f.Owner)
	return prefix, err
}

func LeasePrefixFromFilter(f mv1.LeaseFilters) ([]byte, error) {
	prefix, err := filterToPrefix(buildLeasePrefix(LeasePrefix, f.State), f.Owner, f.DSeq, f.GSeq, f.OSeq, f.Provider, f.BSeq)
	return prefix, err
}

func LeaseReversePrefixFromFilter(f mv1.LeaseFilters) ([]byte, error) {
	prefix, err := reverseFilterToPrefix(buildLeasePrefix(LeasePrefixReverse, f.State), f.Provider, f.BSeq, f.DSeq, f.GSeq, f.OSeq, f.Owner)
	return prefix, err
}
