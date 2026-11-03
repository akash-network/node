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

var chronoIDKey collcodec.KeyCodec[types.ID] = chronologicalCodec{}

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

func statusRange(status types.Status) *collections.Range[types.SettlementID] {
	first := make(sdk.AccAddress, types.AccountLength)
	first[types.AccountLength-1] = 1
	last := sdk.AccAddress(bytes.Repeat([]byte{0xff}, types.AccountLength))
	return new(collections.Range[types.SettlementID]).
		StartInclusive(types.SettlementID{Status: status, Owner: first.String(), Height: 1, Sequence: 1}).
		EndInclusive(types.SettlementID{Status: status, Owner: last.String(), Height: math.MaxInt64, Sequence: math.MaxUint64})
}
