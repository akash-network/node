package keeper

import (
	"encoding/binary"
	"encoding/json"
	"math"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/util/validation"
)

type recordIDCodec struct{}

type settlementIDCodec struct{}

type chronologicalCodec struct{}

var (
	RecordIDKey     = recordIDCodec{}
	SettlementIDKey = settlementIDCodec{}
	chronoIDKey     = chronologicalCodec{}
)

func (recordIDCodec) ToPrefix(key types.ID) ([]byte, error) {
	var buffer []byte
	if key.Owner != "" {
		addr, err := sdk.AccAddressFromBech32(key.Owner)
		if err != nil {
			return nil, err
		}

		buffer, err = validation.EncodeWithLengthPrefix(addr)
		if err != nil {
			return nil, err
		}

		if key.Height > 0 {
			buffer = binary.BigEndian.AppendUint64(buffer, uint64(key.Height)) //nolint:gosec // The prefix height is positive.
			if key.Sequence > 0 {
				buffer = binary.BigEndian.AppendUint64(buffer, key.Sequence)
			}
		}
	}

	return buffer, nil
}

func (recordIDCodec) Encode(buffer []byte, key types.ID) (int, error) {
	addr, err := sdk.AccAddressFromBech32(key.Owner)
	if err != nil {
		return 0, err
	}

	offset, err := sdk.AccAddressKey.EncodeNonTerminal(buffer, addr)
	if err != nil {
		return 0, err
	}

	binary.BigEndian.PutUint64(buffer[offset:], uint64(key.Height)) //nolint:gosec // Internal callers supply validated positive heights.
	offset += 8

	binary.BigEndian.PutUint64(buffer[offset:], key.Sequence)
	offset += 8

	return offset, nil
}

func (recordIDCodec) Decode(buffer []byte) (int, types.ID, error) {
	offset, addr, err := sdk.AccAddressKey.DecodeNonTerminal(buffer)
	if err != nil {
		return 0, types.ID{}, err
	}

	n, height, err := collections.Uint64Key.Decode(buffer[offset:])
	if err != nil {
		return 0, types.ID{}, err
	}

	if height > math.MaxInt64 {
		return 0, types.ID{}, types.ErrInvalidID.Wrap("height out of range")
	}

	offset += n
	n, sequence, err := collections.Uint64Key.Decode(buffer[offset:])
	if err != nil {
		return 0, types.ID{}, err
	}

	offset += n
	key := types.ID{
		Owner:    addr.String(),
		Height:   int64(height),
		Sequence: sequence,
	}
	if err = key.ValidateBasic(); err != nil {
		return 0, types.ID{}, err
	}

	return offset, key, nil
}

func (recordIDCodec) Size(key types.ID) int {
	addr := sdk.MustAccAddressFromBech32(key.Owner)
	return sdk.AccAddressKey.SizeNonTerminal(addr) + 8 + 8
}

func (recordIDCodec) EncodeJSON(key types.ID) ([]byte, error) {
	return json.Marshal(key)
}

func (recordIDCodec) DecodeJSON(b []byte) (types.ID, error) {
	var key types.ID
	if err := json.Unmarshal(b, &key); err != nil {
		return types.ID{}, err
	}

	return key, key.ValidateBasic()
}

func (recordIDCodec) Stringify(key types.ID) string { return key.String() }

func (recordIDCodec) KeyType() string { return "sink.ID" }

func (d recordIDCodec) EncodeNonTerminal(buffer []byte, key types.ID) (int, error) {
	return d.Encode(buffer, key)
}

func (d recordIDCodec) DecodeNonTerminal(buffer []byte) (int, types.ID, error) {
	return d.Decode(buffer)
}

func (d recordIDCodec) SizeNonTerminal(key types.ID) int { return d.Size(key) }

func (settlementIDCodec) ToPrefix(key types.SettlementID) ([]byte, error) {
	if key.Status == types.StatusInvalid {
		return nil, nil
	}

	if !key.Status.IsValid() {
		return nil, types.ErrInvalidSettlement.Wrap("unknown financial status")
	}

	data, err := RecordIDKey.ToPrefix(*key.GetID())
	if err != nil {
		return nil, err
	}

	return append([]byte{byte(key.Status)}, data...), nil //nolint:gosec // IsValid admits only statuses 1 through 5.
}

func (settlementIDCodec) Encode(buffer []byte, key types.SettlementID) (int, error) {
	buffer[0] = byte(key.Status) //nolint:gosec // Internal callers supply validated financial statuses.
	offset := 1
	n, err := RecordIDKey.Encode(buffer[offset:], *key.GetID())
	if err != nil {
		return 0, err
	}

	return offset + n, nil
}

func (settlementIDCodec) Decode(buffer []byte) (int, types.SettlementID, error) {
	if err := validation.KeyAtLeastLength(buffer, 1); err != nil {
		return 0, types.SettlementID{}, err
	}

	status := types.Status(buffer[0])
	if !status.IsValid() {
		return 0, types.SettlementID{}, types.ErrInvalidSettlement.Wrap("unknown financial status")
	}

	offset := 1
	n, id, err := RecordIDKey.Decode(buffer[offset:])
	if err != nil {
		return 0, types.SettlementID{}, err
	}

	return offset + n, types.NewSettlementID(status, id), nil
}

func (settlementIDCodec) Size(key types.SettlementID) int { return 1 + RecordIDKey.Size(*key.GetID()) }

func (settlementIDCodec) EncodeJSON(key types.SettlementID) ([]byte, error) {
	return json.Marshal(key)
}

func (settlementIDCodec) DecodeJSON(b []byte) (types.SettlementID, error) {
	var key types.SettlementID
	if err := json.Unmarshal(b, &key); err != nil {
		return types.SettlementID{}, err
	}

	return key, key.ValidateBasic()
}

func (settlementIDCodec) Stringify(key types.SettlementID) string {
	return key.Status.String() + "/" + key.GetID().String()
}

func (settlementIDCodec) KeyType() string { return "sink.SettlementID" }

func (d settlementIDCodec) EncodeNonTerminal(buffer []byte, key types.SettlementID) (int, error) {
	return d.Encode(buffer, key)
}

func (d settlementIDCodec) DecodeNonTerminal(buffer []byte) (int, types.SettlementID, error) {
	return d.Decode(buffer)
}

func (d settlementIDCodec) SizeNonTerminal(key types.SettlementID) int { return d.Size(key) }

func (chronologicalCodec) Encode(buffer []byte, key types.ID) (int, error) {
	offset := 0
	binary.BigEndian.PutUint64(buffer[offset:], uint64(key.Height)) //nolint:gosec // Internal callers supply validated positive heights.
	offset += 8

	binary.BigEndian.PutUint64(buffer[offset:], key.Sequence)
	offset += 8

	addr, err := sdk.AccAddressFromBech32(key.Owner)
	if err != nil {
		return 0, err
	}

	n, err := sdk.AccAddressKey.EncodeNonTerminal(buffer[offset:], addr)
	if err != nil {
		return 0, err
	}

	return offset + n, nil
}

func (chronologicalCodec) Decode(buffer []byte) (int, types.ID, error) {
	offset, height, err := collections.Uint64Key.Decode(buffer)
	if err != nil {
		return 0, types.ID{}, err
	}

	if height > math.MaxInt64 {
		return 0, types.ID{}, types.ErrInvalidID.Wrap("height out of range")
	}

	n, sequence, err := collections.Uint64Key.Decode(buffer[offset:])
	if err != nil {
		return 0, types.ID{}, err
	}

	offset += n
	n, addr, err := sdk.AccAddressKey.DecodeNonTerminal(buffer[offset:])
	if err != nil {
		return 0, types.ID{}, err
	}

	offset += n
	key := types.ID{
		Owner:    addr.String(),
		Height:   int64(height),
		Sequence: sequence,
	}
	if err = key.ValidateBasic(); err != nil {
		return 0, types.ID{}, err
	}

	return offset, key, nil
}

func (chronologicalCodec) Size(key types.ID) int { return RecordIDKey.Size(key) }

func (chronologicalCodec) EncodeJSON(key types.ID) ([]byte, error) {
	return json.Marshal(key)
}

func (chronologicalCodec) DecodeJSON(b []byte) (types.ID, error) {
	return RecordIDKey.DecodeJSON(b)
}

func (chronologicalCodec) Stringify(key types.ID) string { return key.String() }

func (chronologicalCodec) KeyType() string { return "chronological_sink_id" }

func (d chronologicalCodec) EncodeNonTerminal(buffer []byte, key types.ID) (int, error) {
	return d.Encode(buffer, key)
}

func (d chronologicalCodec) DecodeNonTerminal(buffer []byte) (int, types.ID, error) {
	return d.Decode(buffer)
}

func (d chronologicalCodec) SizeNonTerminal(key types.ID) int { return d.Size(key) }
