package keeper

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"cosmossdk.io/collections/codec"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	"pkg.akt.dev/go/util/conv"

	types "pkg.akt.dev/go/node/bme/v1"

	"pkg.akt.dev/node/v2/util/validation"
)

type ledgerRecordIDCodec struct{}

var (
	LedgerRecordIDKey codec.KeyCodec[types.LedgerRecordID] = ledgerRecordIDCodec{}
)

func (d ledgerRecordIDCodec) ToPrefix(key types.LedgerRecordID) ([]byte, error) {
	buffer := bytes.Buffer{}

	if key.Denom != "" {
		data := conv.UnsafeStrToBytes(key.Denom)
		buffer.WriteByte(byte(len(data)))
		buffer.Write(data)

		if key.ToDenom != "" {
			data = conv.UnsafeStrToBytes(key.ToDenom)
			buffer.WriteByte(byte(len(data)))
			buffer.Write(data)

			if key.Source != "" {
				addr, err := sdktypes.AccAddressFromBech32(key.Source)
				if err != nil {
					return nil, err
				}

				data, err = validation.EncodeWithLengthPrefix(addr)
				if err != nil {
					return nil, err
				}

				buffer.Write(data)

				if key.Height > 0 {
					data = make([]byte, 8)
					binary.BigEndian.PutUint64(data, uint64(key.Height))
					buffer.Write(data)

					if key.Sequence > 0 {
						data = make([]byte, 8)
						binary.BigEndian.PutUint64(data, uint64(key.Sequence))
						buffer.Write(data)
					}
				}
			}
		}
	}

	return buffer.Bytes(), nil
}

func (d ledgerRecordIDCodec) Encode(buffer []byte, key types.LedgerRecordID) (int, error) {
	offset := 0

	data := conv.UnsafeStrToBytes(key.Denom)
	buffer[offset] = byte(len(data))
	offset++
	offset += copy(buffer[offset:], data)

	data = conv.UnsafeStrToBytes(key.ToDenom)
	buffer[offset] = byte(len(data))
	offset++
	offset += copy(buffer[offset:], data)

	addr, err := sdktypes.AccAddressFromBech32(key.Source)
	if err != nil {
		return 0, err
	}

	data, err = validation.EncodeWithLengthPrefix(addr)
	if err != nil {
		return 0, err
	}

	offset += copy(buffer[offset:], data)

	binary.BigEndian.PutUint64(buffer[offset:], uint64(key.Height))
	offset += 8

	binary.BigEndian.PutUint64(buffer[offset:], uint64(key.Sequence))
	offset += 8

	return offset, nil
}

func (d ledgerRecordIDCodec) Decode(buffer []byte) (int, types.LedgerRecordID, error) {
	originBuffer := buffer

	err := validation.KeyAtLeastLength(buffer, 5)
	if err != nil {
		return 0, types.LedgerRecordID{}, err
	}

	res := types.LedgerRecordID{}

	// decode denom
	dataLen := int(buffer[0])
	buffer = buffer[1:]

	err = validation.KeyAtLeastLength(buffer, dataLen)
	if err != nil {
		return 0, types.LedgerRecordID{}, err
	}

	res.Denom = conv.UnsafeBytesToStr(buffer[:dataLen])
	buffer = buffer[dataLen:]

	err = validation.KeyAtLeastLength(buffer, 1)
	if err != nil {
		return 0, types.LedgerRecordID{}, err
	}

	// decode base denom
	dataLen = int(buffer[0])
	buffer = buffer[1:]

	err = validation.KeyAtLeastLength(buffer, dataLen)
	if err != nil {
		return 0, types.LedgerRecordID{}, err
	}

	res.ToDenom = conv.UnsafeBytesToStr(buffer[:dataLen])
	buffer = buffer[dataLen:]

	// decode address
	err = validation.KeyAtLeastLength(buffer, 1)
	if err != nil {
		return 0, types.LedgerRecordID{}, err
	}

	dataLen = int(buffer[0])
	buffer = buffer[1:]

	addr := sdktypes.AccAddress(buffer[:dataLen])
	res.Source = addr.String()
	buffer = buffer[dataLen:]

	// decode height
	err = validation.KeyAtLeastLength(buffer, 8)
	if err != nil {
		return 0, types.LedgerRecordID{}, err
	}

	res.Height = int64(binary.BigEndian.Uint64(buffer))
	buffer = buffer[8:]

	// decode sequence
	err = validation.KeyAtLeastLength(buffer, 8)
	if err != nil {
		return 0, types.LedgerRecordID{}, err
	}

	res.Sequence = int64(binary.BigEndian.Uint64(buffer))
	buffer = buffer[8:]

	return len(originBuffer) - len(buffer), res, nil
}

func (d ledgerRecordIDCodec) Size(key types.LedgerRecordID) int {
	size := 0
	if key.Denom != "" {
		size += len(conv.UnsafeStrToBytes(key.Denom)) + 1

		if key.ToDenom != "" {
			size += len(conv.UnsafeStrToBytes(key.ToDenom)) + 1

			if key.Source != "" {
				addr := sdktypes.MustAccAddressFromBech32(key.Source)
				size += 1 + len(addr)

				if key.Height > 0 {
					size += 8

					if key.Sequence > 0 {
						size += 8
					}
				}
			}
		}
	}

	return size
}

func (d ledgerRecordIDCodec) EncodeJSON(key types.LedgerRecordID) ([]byte, error) {
	return json.Marshal(key)
}

func (d ledgerRecordIDCodec) DecodeJSON(b []byte) (types.LedgerRecordID, error) {
	var key types.LedgerRecordID
	err := json.Unmarshal(b, &key)
	return key, err
}

func (d ledgerRecordIDCodec) Stringify(key types.LedgerRecordID) string {
	return fmt.Sprintf("%s/%s/%s/%d/%d", key.Denom, key.ToDenom, key.Source, key.Height, key.Sequence)
}

func (d ledgerRecordIDCodec) KeyType() string {
	return "LedgerRecordID"
}

// NonTerminal variants - for use in composite keys
// Must use length-prefixing or fixed-size encoding

func (d ledgerRecordIDCodec) EncodeNonTerminal(buffer []byte, key types.LedgerRecordID) (int, error) {
	return d.Encode(buffer, key)
}

func (d ledgerRecordIDCodec) DecodeNonTerminal(buffer []byte) (int, types.LedgerRecordID, error) {
	return d.Decode(buffer)
}

func (d ledgerRecordIDCodec) SizeNonTerminal(key types.LedgerRecordID) int {
	return d.Size(key)
}
