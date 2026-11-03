package keeper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"

	"cosmossdk.io/collections"
	collcodec "cosmossdk.io/collections/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/sink/v1"
)

var (
	chronoIDKey     collcodec.KeyCodec[types.ID]           = chronologicalCodec{}
	SettlementIDKey collcodec.KeyCodec[types.SettlementID] = settlementIDCodec{}
)

type chronologicalCodec struct{}

func (chronologicalCodec) Encode(buf []byte, id types.ID) (int, error) {
	if len(buf) < types.IDKeySize {
		return 0, fmt.Errorf("short chronological key buffer")
	}

	var ownerFirst [types.IDKeySize]byte
	n, err := types.IDKey.Encode(ownerFirst[:], id)
	if err != nil {
		return 0, err
	}

	copy(buf[:16], ownerFirst[21:])
	copy(buf[16:37], ownerFirst[:21])

	return n, nil
}

func (chronologicalCodec) Decode(buf []byte) (int, types.ID, error) {
	if len(buf) < types.IDKeySize {
		return 0, types.ID{}, fmt.Errorf("short chronological key")
	}

	var ownerFirst [types.IDKeySize]byte
	copy(ownerFirst[:21], buf[16:37])
	copy(ownerFirst[21:], buf[:16])

	return types.IDKey.Decode(ownerFirst[:])
}

func (chronologicalCodec) Size(types.ID) int { return types.IDKeySize }

func (chronologicalCodec) EncodeJSON(id types.ID) ([]byte, error) {
	return json.Marshal(id)
}

func (chronologicalCodec) DecodeJSON(b []byte) (types.ID, error) {
	var id types.ID
	if err := json.Unmarshal(b, &id); err != nil {
		return id, err
	}

	return id, id.ValidateBasic()
}

func (chronologicalCodec) Stringify(id types.ID) string { return id.String() }

func (chronologicalCodec) KeyType() string { return "chronological_sink_id" }

func (c chronologicalCodec) EncodeNonTerminal(b []byte, id types.ID) (int, error) {
	return c.Encode(b, id)
}

func (c chronologicalCodec) DecodeNonTerminal(b []byte) (int, types.ID, error) {
	return c.Decode(b)
}

func (c chronologicalCodec) SizeNonTerminal(id types.ID) int {
	return c.Size(id)
}

type settlementIDCodec struct{}

const settlementIDSize = 1 + types.IDKeySize

func settlementID(status types.Status, id types.ID) types.SettlementID {
	return types.SettlementID{Status: status, Owner: id.Owner, Height: id.Height, Sequence: id.Sequence}
}

func transferID(key types.SettlementID) types.ID {
	return types.ID{Owner: key.Owner, Height: key.Height, Sequence: key.Sequence}
}

func statusByte(status types.Status) (byte, error) {
	if !status.IsValid() {
		return 0, types.ErrInvalidSettlement.Wrap("unknown financial status")
	}

	return byte(status), nil //nolint:gosec // IsValid admits only statuses 1, 2, 4 and 5.
}

func statusRange(status types.Status) *collections.Range[types.SettlementID] {
	first := make(sdk.AccAddress, types.AccountLength)
	first[types.AccountLength-1] = 1
	last := sdk.AccAddress(bytes.Repeat([]byte{0xff}, types.AccountLength))
	return new(collections.Range[types.SettlementID]).
		StartInclusive(types.SettlementID{Status: status, Owner: first.String(), Height: 1, Sequence: 1}).
		EndInclusive(types.SettlementID{Status: status, Owner: last.String(), Height: math.MaxInt64, Sequence: math.MaxUint64})
}

func (settlementIDCodec) Encode(b []byte, key types.SettlementID) (int, error) {
	if len(b) < settlementIDSize {
		return 0, types.ErrInvalidID.Wrap("short settlement key buffer")
	}

	status, err := statusByte(key.Status)
	if err != nil {
		return 0, err
	}

	n, err := types.IDKey.Encode(b[1:], transferID(key))
	if err != nil {
		return 0, err
	}

	b[0] = status

	return 1 + n, nil
}

func (settlementIDCodec) Decode(b []byte) (int, types.SettlementID, error) {
	if len(b) < settlementIDSize {
		return 0, types.SettlementID{}, types.ErrInvalidID.Wrap("short settlement key")
	}

	status := types.Status(b[0])
	if _, err := statusByte(status); err != nil {
		return 0, types.SettlementID{}, err
	}

	n, id, err := types.IDKey.Decode(b[1:])
	if err != nil {
		return 0, types.SettlementID{}, err
	}

	return 1 + n, settlementID(status, id), nil
}

func (settlementIDCodec) Size(types.SettlementID) int { return settlementIDSize }

func (c settlementIDCodec) EncodeNonTerminal(b []byte, key types.SettlementID) (int, error) {
	return c.Encode(b, key)
}

func (c settlementIDCodec) DecodeNonTerminal(b []byte) (int, types.SettlementID, error) {
	return c.Decode(b)
}

func (settlementIDCodec) SizeNonTerminal(types.SettlementID) int { return settlementIDSize }

func (settlementIDCodec) KeyType() string { return "sink.SettlementID" }

func (settlementIDCodec) Stringify(key types.SettlementID) string {
	return key.Status.String() + "/" + transferID(key).String()
}

func (settlementIDCodec) EncodeJSON(key types.SettlementID) ([]byte, error) {
	if _, err := statusByte(key.Status); err != nil {
		return nil, err
	}

	if err := transferID(key).ValidateBasic(); err != nil {
		return nil, err
	}

	return json.Marshal(key)
}

func (settlementIDCodec) DecodeJSON(b []byte) (types.SettlementID, error) {
	var key types.SettlementID
	if err := json.Unmarshal(b, &key); err != nil {
		return types.SettlementID{}, err
	}

	if _, err := statusByte(key.Status); err != nil {
		return types.SettlementID{}, err
	}

	return key, transferID(key).ValidateBasic()
}
