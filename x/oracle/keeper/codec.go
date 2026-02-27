package keeper

import (
	"encoding/binary"
	"encoding/json"
	"fmt"

	"cosmossdk.io/collections/codec"
	types "pkg.akt.dev/go/node/oracle/v1"
	"pkg.akt.dev/go/util/conv"

	"pkg.akt.dev/node/v2/util/validation"
)

// priceDataIDCodec implements codec.KeyCodec[PriceDataID]
type priceDataIDCodec struct{}

type dataIDCodec struct{}

// priceDataRecordIDCodec implements codec.KeyCodec[PriceDataID]
type priceDataRecordIDCodec struct{}

// PriceDataRecordIDKey is the codec instance to use when creating a Map
var (
	PriceDataIDKey       codec.KeyCodec[types.PriceDataID]       = priceDataIDCodec{}
	DataIDKey            codec.KeyCodec[types.DataID]            = dataIDCodec{}
	PriceDataRecordIDKey codec.KeyCodec[types.PriceDataRecordID] = priceDataRecordIDCodec{}
)

func (d priceDataIDCodec) Encode(buffer []byte, key types.PriceDataID) (int, error) {
	offset := 0
	// Write source id as big-endian uint64 (for proper ordering)
	binary.BigEndian.PutUint32(buffer, key.Source)
	offset += 4

	data := conv.UnsafeStrToBytes(key.Denom)
	buffer[offset] = byte(len(data))
	offset++

	offset += copy(buffer[offset:], data)

	data = conv.UnsafeStrToBytes(key.BaseDenom)
	buffer[offset] = byte(len(data))
	offset++

	offset += copy(buffer[offset:], data)

	return offset, nil
}

func (d priceDataIDCodec) Decode(buffer []byte) (int, types.PriceDataID, error) {
	err := validation.KeyAtLeastLength(buffer, 5)
	if err != nil {
		return 0, types.PriceDataID{}, err
	}

	res := types.PriceDataID{}

	res.Source = binary.BigEndian.Uint32(buffer)

	buffer = buffer[4:]

	dataLen := int(buffer[0])
	buffer = buffer[1:]

	decodedLen := 4 + 1 + dataLen

	err = validation.KeyAtLeastLength(buffer, dataLen)
	if err != nil {
		return 0, types.PriceDataID{}, err
	}

	res.Denom = conv.UnsafeBytesToStr(buffer[:dataLen])
	buffer = buffer[dataLen:]

	err = validation.KeyAtLeastLength(buffer, 1)
	if err != nil {
		return 0, types.PriceDataID{}, err
	}

	dataLen = int(buffer[0])
	buffer = buffer[1:]

	decodedLen += 1 + dataLen

	err = validation.KeyAtLeastLength(buffer, dataLen)
	if err != nil {
		return 0, types.PriceDataID{}, err
	}

	res.BaseDenom = conv.UnsafeBytesToStr(buffer[:dataLen])

	return decodedLen, res, nil
}

func (d priceDataIDCodec) Size(key types.PriceDataID) int {
	ln := len(conv.UnsafeStrToBytes(key.Denom)) + 1
	ln += len(conv.UnsafeStrToBytes(key.BaseDenom)) + 1

	return 4 + ln
}

func (d priceDataIDCodec) EncodeJSON(key types.PriceDataID) ([]byte, error) {
	return json.Marshal(key)
}

func (d priceDataIDCodec) DecodeJSON(b []byte) (types.PriceDataID, error) {
	var key types.PriceDataID
	err := json.Unmarshal(b, &key)
	return key, err
}

func (d priceDataIDCodec) Stringify(key types.PriceDataID) string {
	return fmt.Sprintf("%d/%s/%s", key.Source, key.Denom, key.BaseDenom)
}

func (d priceDataIDCodec) KeyType() string {
	return "PriceDataID"
}

// NonTerminal variants - for use in composite keys
// Must use length-prefixing or fixed-size encoding

func (d priceDataIDCodec) EncodeNonTerminal(buffer []byte, key types.PriceDataID) (int, error) {
	return d.Encode(buffer, key)
}

func (d priceDataIDCodec) DecodeNonTerminal(buffer []byte) (int, types.PriceDataID, error) {
	return d.Decode(buffer)
}

func (d priceDataIDCodec) SizeNonTerminal(key types.PriceDataID) int {
	return d.Size(key)
}

func (d dataIDCodec) Encode(buffer []byte, key types.DataID) (int, error) {
	offset := 0

	data := conv.UnsafeStrToBytes(key.Denom)
	buffer[offset] = byte(len(data))
	offset++

	offset += copy(buffer[offset:], data)

	data = conv.UnsafeStrToBytes(key.BaseDenom)
	buffer[offset] = byte(len(data))
	offset++

	offset += copy(buffer[offset:], data)

	return offset, nil
}

func (d dataIDCodec) Decode(buffer []byte) (int, types.DataID, error) {
	err := validation.KeyAtLeastLength(buffer, 1)
	if err != nil {
		return 0, types.DataID{}, err
	}

	res := types.DataID{}

	dataLen := int(buffer[0])
	buffer = buffer[1:]

	decodedLen := 1 + dataLen

	err = validation.KeyAtLeastLength(buffer, dataLen)
	if err != nil {
		return 0, types.DataID{}, err
	}

	res.Denom = conv.UnsafeBytesToStr(buffer[:dataLen])
	buffer = buffer[dataLen:]

	err = validation.KeyAtLeastLength(buffer, 1)
	if err != nil {
		return 0, types.DataID{}, err
	}

	dataLen = int(buffer[0])
	buffer = buffer[1:]

	decodedLen += 1 + dataLen

	err = validation.KeyAtLeastLength(buffer, dataLen)
	if err != nil {
		return 0, types.DataID{}, err
	}

	res.BaseDenom = conv.UnsafeBytesToStr(buffer[:dataLen])

	return decodedLen, res, nil
}

func (d dataIDCodec) Size(key types.DataID) int {
	ln := len(conv.UnsafeStrToBytes(key.Denom)) + 1
	ln += len(conv.UnsafeStrToBytes(key.BaseDenom)) + 1

	return ln
}

func (d dataIDCodec) EncodeJSON(key types.DataID) ([]byte, error) {
	return json.Marshal(key)
}

func (d dataIDCodec) DecodeJSON(b []byte) (types.DataID, error) {
	var key types.DataID
	err := json.Unmarshal(b, &key)
	return key, err
}

func (d dataIDCodec) Stringify(key types.DataID) string {
	return fmt.Sprintf("%s/%s", key.Denom, key.BaseDenom)
}

func (d dataIDCodec) KeyType() string {
	return "AggregatedDataID"
}

// NonTerminal variants - for use in composite keys
// Must use length-prefixing or fixed-size encoding

func (d dataIDCodec) EncodeNonTerminal(buffer []byte, key types.DataID) (int, error) {
	return d.Encode(buffer, key)
}

func (d dataIDCodec) DecodeNonTerminal(buffer []byte) (int, types.DataID, error) {
	return d.Decode(buffer)
}

func (d dataIDCodec) SizeNonTerminal(key types.DataID) int {
	return d.Size(key)
}

func (d priceDataRecordIDCodec) Encode(buffer []byte, key types.PriceDataRecordID) (int, error) {
	offset := 0
	// Write source id as big-endian uint64 (for proper ordering)
	binary.BigEndian.PutUint32(buffer, key.Source)
	offset += 4

	data := conv.UnsafeStrToBytes(key.Denom)
	buffer[offset] = byte(len(data))
	offset++

	offset += copy(buffer[offset:], data)

	data = conv.UnsafeStrToBytes(key.BaseDenom)
	buffer[offset] = byte(len(data))
	offset++

	offset += copy(buffer[offset:], data)

	binary.BigEndian.PutUint64(buffer[offset:], uint64(key.Height))
	offset += 8

	return offset, nil
}

func (d priceDataRecordIDCodec) Decode(buffer []byte) (int, types.PriceDataRecordID, error) {
	err := validation.KeyAtLeastLength(buffer, 5)
	if err != nil {
		return 0, types.PriceDataRecordID{}, err
	}

	res := types.PriceDataRecordID{}

	res.Source = binary.BigEndian.Uint32(buffer)

	buffer = buffer[4:]

	dataLen := int(buffer[0])
	buffer = buffer[1:]

	decodedLen := 4 + 1 + dataLen

	err = validation.KeyAtLeastLength(buffer, dataLen)
	if err != nil {
		return 0, types.PriceDataRecordID{}, err
	}

	res.Denom = conv.UnsafeBytesToStr(buffer[:dataLen])
	buffer = buffer[dataLen:]

	err = validation.KeyAtLeastLength(buffer, 1)
	if err != nil {
		return 0, types.PriceDataRecordID{}, err
	}

	dataLen = int(buffer[0])
	buffer = buffer[1:]

	decodedLen += 1 + dataLen

	err = validation.KeyAtLeastLength(buffer, dataLen)
	if err != nil {
		return 0, types.PriceDataRecordID{}, err
	}

	res.BaseDenom = conv.UnsafeBytesToStr(buffer[:dataLen])
	buffer = buffer[dataLen:]

	err = validation.KeyAtLeastLength(buffer, 8)
	if err != nil {
		return 0, types.PriceDataRecordID{}, err
	}

	res.Height = int64(binary.BigEndian.Uint64(buffer))

	decodedLen += 8

	return decodedLen, res, nil
}

func (d priceDataRecordIDCodec) Size(key types.PriceDataRecordID) int {
	ln := len(conv.UnsafeStrToBytes(key.Denom)) + 1
	ln += len(conv.UnsafeStrToBytes(key.BaseDenom)) + 1

	return 4 + ln + 8
}

func (d priceDataRecordIDCodec) EncodeJSON(key types.PriceDataRecordID) ([]byte, error) {
	return json.Marshal(key)
}

func (d priceDataRecordIDCodec) DecodeJSON(b []byte) (types.PriceDataRecordID, error) {
	var key types.PriceDataRecordID
	err := json.Unmarshal(b, &key)
	return key, err
}

func (d priceDataRecordIDCodec) Stringify(key types.PriceDataRecordID) string {
	return fmt.Sprintf("%d/%s/%s/%d", key.Source, key.Denom, key.BaseDenom, key.Height)
}

func (d priceDataRecordIDCodec) KeyType() string {
	return "PriceDataRecordID"
}

// NonTerminal variants - for use in composite keys
// Must use length-prefixing or fixed-size encoding

func (d priceDataRecordIDCodec) EncodeNonTerminal(buffer []byte, key types.PriceDataRecordID) (int, error) {
	return d.Encode(buffer, key)
}

func (d priceDataRecordIDCodec) DecodeNonTerminal(buffer []byte) (int, types.PriceDataRecordID, error) {
	return d.Decode(buffer)
}

func (d priceDataRecordIDCodec) SizeNonTerminal(key types.PriceDataRecordID) int {
	return d.Size(key)
}
