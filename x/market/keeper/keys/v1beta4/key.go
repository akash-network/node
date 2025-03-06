package v1beta4

import (
	"bytes"
	"encoding/binary"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"

	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	types "github.com/akash-network/akash-api/go/node/market/v1beta4"
	"github.com/akash-network/akash-api/go/sdkutil"
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
	OrderStateOpenPrefix              = []byte{OrderStateOpenPrefixID}
	OrderStateActivePrefix            = []byte{OrderStateActivePrefixID}
	OrderStateClosedPrefix            = []byte{OrderStateClosedPrefixID}
	BidPrefix                         = []byte{0x12, 0x00}
	BidPrefixReverse                  = []byte{0x12, 0x01}
	BidStateOpenPrefix                = []byte{BidStateOpenPrefixID}
	BidStateActivePrefix              = []byte{BidStateActivePrefixID}
	BidStateLostPrefix                = []byte{BidStateLostPrefixID}
	BidStateClosedPrefix              = []byte{BidStateClosedPrefixID}
	LeasePrefix                       = []byte{0x13, 0x00}
	LeasePrefixReverse                = []byte{0x13, 0x01}
	LeaseStateActivePrefix            = []byte{LeaseStateActivePrefixID}
	LeaseStateInsufficientFundsPrefix = []byte{LeaseStateInsufficientFundsPrefixID}
	LeaseStateClosedPrefix            = []byte{LeaseStateClosedPrefixID}
)

func OrderKey(statePrefix []byte, id types.OrderID) ([]byte, error) {
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

func MustOrderKey(statePrefix []byte, id types.OrderID) []byte {
	key, err := OrderKey(statePrefix, id)
	if err != nil {
		panic(err)
	}
	return key
}

func BidKey(statePrefix []byte, id types.BidID) ([]byte, error) {
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

	return buf.Bytes(), nil
}

func MustBidKey(statePrefix []byte, id types.BidID) []byte {
	key, err := BidKey(statePrefix, id)
	if err != nil {
		panic(err)
	}
	return key
}

func BidReverseKey(statePrefix []byte, id types.BidID) ([]byte, error) {
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

func MustBidReverseKey(statePrefix []byte, id types.BidID) []byte {
	key, err := BidReverseKey(statePrefix, id)
	if err != nil {
		panic(err)
	}
	return key
}

func BidStateReverseKey(state types.Bid_State, id types.BidID) ([]byte, error) {
	if state != types.BidActive && state != types.BidOpen {
		return nil, nil
	}

	prefix := BidStateToPrefix(state)
	key, err := BidReverseKey(prefix, id)
	if err != nil {
		return nil, err
	}

	return key, nil
}

func MustBidStateRevereKey(state types.Bid_State, id types.BidID) []byte {
	key, err := BidStateReverseKey(state, id)
	if err != nil {
		panic(err)
	}

	return key
}

func LeaseKey(statePrefix []byte, id types.LeaseID) ([]byte, error) {
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

	return buf.Bytes(), nil
}

func MustLeaseKey(statePrefix []byte, id types.LeaseID) []byte {
	key, err := LeaseKey(statePrefix, id)
	if err != nil {
		panic(err)
	}
	return key
}

func LeaseReverseKey(statePrefix []byte, id types.LeaseID) ([]byte, error) {
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

func LeaseStateReverseKey(state types.Lease_State, id types.LeaseID) ([]byte, error) {
	if state != types.LeaseActive {
		return nil, nil
	}

	prefix := LeaseStateToPrefix(state)
	key, err := LeaseReverseKey(prefix, id)
	if err != nil {
		return nil, err
	}

	return key, nil
}

func MustLeaseStateReverseKey(state types.Lease_State, id types.LeaseID) []byte {
	key, err := LeaseStateReverseKey(state, id)
	if err != nil {
		panic(err)
	}

	return key
}

func MustLeaseReverseKey(statePrefix []byte, id types.LeaseID) []byte {
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

func BidsForOrderPrefix(statePrefix []byte, id types.OrderID) []byte {
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

func OrderStateToPrefix(state types.Order_State) []byte {
	var res []byte

	switch state {
	case types.OrderOpen:
		res = OrderStateOpenPrefix
	case types.OrderActive:
		res = OrderStateActivePrefix
	case types.OrderClosed:
		res = OrderStateClosedPrefix
	}

	return res
}

func BidStateToPrefix(state types.Bid_State) []byte {
	var res []byte

	switch state {
	case types.BidOpen:
		res = BidStateOpenPrefix
	case types.BidActive:
		res = BidStateActivePrefix
	case types.BidLost:
		res = BidStateLostPrefix
	case types.BidClosed:
		res = BidStateClosedPrefix
	}

	return res
}

func LeaseStateToPrefix(state types.Lease_State) []byte {
	var res []byte

	switch state {
	case types.LeaseActive:
		res = LeaseStateActivePrefix
	case types.LeaseInsufficientFunds:
		res = LeaseStateInsufficientFundsPrefix
	case types.LeaseClosed:
		res = LeaseStateClosedPrefix
	}

	return res
}

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

// nolint: unused
func reverseFilterToPrefix(prefix []byte, provider string, dseq uint64, gseq, oseq uint32, owner string) ([]byte, error) {
	buf := bytes.NewBuffer(prefix)

	if len(provider) == 0 {
		return buf.Bytes(), nil
	}

	if _, err := buf.Write(address.MustLengthPrefix(sdkutil.MustAccAddressFromBech32(provider))); err != nil {
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

func OrderPrefixFromFilter(f types.OrderFilters) ([]byte, error) {
	var idx []byte
	switch f.State {
	case types.OrderOpen.String():
		idx = OrderStateOpenPrefix
	case types.OrderActive.String():
		idx = OrderStateActivePrefix
	case types.OrderClosed.String():
		idx = OrderStateClosedPrefix
	}

	prefix := make([]byte, 0, len(OrderPrefix)+len(idx))
	prefix = append(prefix, OrderPrefix...)
	prefix = append(prefix, idx...)

	return filterToPrefix(prefix, f.Owner, f.DSeq, f.GSeq, f.OSeq, "")
}

func buildLeasePrefix(prefix []byte, state string) []byte {
	var idx []byte
	switch state {
	case types.LeaseActive.String():
		idx = LeaseStateActivePrefix
	case types.LeaseInsufficientFunds.String():
		idx = LeaseStateInsufficientFundsPrefix
	case types.LeaseClosed.String():
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
	case types.BidActive.String():
		idx = BidStateActivePrefix
	case types.BidOpen.String():
		idx = BidStateOpenPrefix
	case types.BidLost.String():
		idx = BidStateLostPrefix
	case types.BidClosed.String():
		idx = BidStateClosedPrefix
	}

	res := make([]byte, 0, len(prefix)+len(idx))
	res = append(res, prefix...)
	res = append(res, idx...)

	return res
}

func BidPrefixFromFilter(f types.BidFilters) ([]byte, error) {
	return filterToPrefix(buildBidPrefix(BidPrefix, f.State), f.Owner, f.DSeq, f.GSeq, f.OSeq, f.Provider)
}

func BidReversePrefixFromFilter(f types.BidFilters) ([]byte, error) {
	prefix, err := filterToPrefix(buildBidPrefix(BidPrefixReverse, f.State), f.Provider, f.DSeq, f.GSeq, f.OSeq, f.Owner)
	return prefix, err
}

func LeasePrefixFromFilter(f types.LeaseFilters) ([]byte, error) {
	prefix, err := filterToPrefix(buildLeasePrefix(LeasePrefix, f.State), f.Owner, f.DSeq, f.GSeq, f.OSeq, f.Provider)
	return prefix, err
}

func LeaseReversePrefixFromFilter(f types.LeaseFilters) ([]byte, error) {
	prefix, err := filterToPrefix(buildLeasePrefix(LeasePrefixReverse, f.State), f.Provider, f.DSeq, f.GSeq, f.OSeq, f.Owner)
	return prefix, err
}

func OrderKeyLegacy(id types.OrderID) []byte {
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

func BidKeyLegacy(id types.BidID) []byte {
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

func LeaseKeyLegacy(id types.LeaseID) []byte {
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

func SecondaryLeaseKeyByProviderLegacy(id types.LeaseID) []byte {
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

func SecondaryKeysForLeaseLegacy(id types.LeaseID) [][]byte {
	return [][]byte{
		SecondaryLeaseKeyByProviderLegacy(id),
	}
}

func OrdersForGroupPrefixLegacy(id dtypes.GroupID) []byte {
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

func BidsForOrderPrefixLegacy(id types.OrderID) []byte {
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
