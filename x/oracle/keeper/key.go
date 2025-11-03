package keeper

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"cosmossdk.io/collections"
	types "pkg.akt.dev/go/node/oracle/v1"
	"pkg.akt.dev/go/util/conv"

	"pkg.akt.dev/node/v2/util/validation"
)

var (
	PricesPrefix       = []byte{0x11, 0x00}
	LatestPricesID     = byte(0x01)
	LatestPricesPrefix = []byte{0x12, LatestPricesID}

	ParamsKey = collections.NewPrefix(9) // key for oracle module params
)

func BuildPricePrefix(assetDenom string, baseDenom string, height int64) ([]byte, error) {
	buf := bytes.NewBuffer(PricesPrefix)

	if assetDenom != "" {
		data := conv.UnsafeStrToBytes(assetDenom)

		buf.WriteByte(byte(len(data)))
		buf.Write(data)

		if baseDenom != "" {
			data = conv.UnsafeStrToBytes(baseDenom)

			buf.WriteByte(byte(len(data)))
			buf.Write(data)

			if height > 0 {
				data = make([]byte, 0)
				dataLen := binary.PutVarint(data, height)

				buf.WriteByte(byte(dataLen))
				buf.Write(data)
			}
		}
	}

	return buf.Bytes(), nil
}

func MustBuildPricePrefix(assetDenom string, baseDenom string, height int64) []byte {
	res, err := BuildPricePrefix(assetDenom, baseDenom, height)
	if err != nil {
		panic(err)
	}

	return res
}

func BuildPriceLatestHeightPrefix(baseDenom string, assetDenom string) ([]byte, error) {
	buf := bytes.NewBuffer(LatestPricesPrefix)

	if assetDenom != "" {
		data := conv.UnsafeStrToBytes(assetDenom)

		buf.WriteByte(byte(len(data)))
		buf.Write(data)

		if baseDenom != "" {
			data = conv.UnsafeStrToBytes(baseDenom)

			buf.WriteByte(byte(len(data)))
			buf.Write(data)
		}
	}

	return buf.Bytes(), nil
}

func MustBuildPriceLatestHeightPrefix(assetDenom string, baseDenom string) []byte {
	res, err := BuildPriceLatestHeightPrefix(assetDenom, baseDenom)
	if err != nil {
		panic(err)
	}

	return res
}

func ParsePriceEntryID(key []byte) (types.PriceEntryID, error) {
	err := validation.KeyAtLeastLength(key, len(PricesPrefix)+1)
	if err != nil {
		return types.PriceEntryID{}, err
	}

	if !bytes.HasPrefix(key, PricesPrefix) {
		return types.PriceEntryID{}, fmt.Errorf("invalid key prefix. expected 0x%s, actual 0x%s", hex.EncodeToString(PricesPrefix), hex.EncodeToString(key[:2]))
	}

	key = key[len(PricesPrefix):]
	dataLen := int(key[0])
	key = key[1:]

	if err = validation.KeyAtLeastLength(key, dataLen); err != nil {
		return types.PriceEntryID{}, err
	}

	assetDenom := conv.UnsafeBytesToStr(key[:dataLen])

	if err = validation.KeyAtLeastLength(key, 1); err != nil {
		return types.PriceEntryID{}, err
	}

	dataLen = int(key[0])
	key = key[1:]

	if err = validation.KeyAtLeastLength(key, dataLen); err != nil {
		return types.PriceEntryID{}, err
	}

	baseDenom := conv.UnsafeBytesToStr(key[:dataLen])

	if err = validation.KeyAtLeastLength(key, 1); err != nil {
		return types.PriceEntryID{}, err
	}

	dataLen = int(key[0])
	key = key[1:]

	if err = validation.KeyAtLeastLength(key, dataLen); err != nil {
		return types.PriceEntryID{}, err
	}

	height, n := binary.Varint(key)
	key = key[n:]

	if err = validation.KeyLength(key, 0); err != nil {
		return types.PriceEntryID{}, err
	}

	return types.PriceEntryID{
		AssetDenom: assetDenom,
		BaseDenom:  baseDenom,
		Height:     height,
	}, nil
}

func MustParsePriceEntryID(key []byte) types.PriceEntryID {
	id, err := ParsePriceEntryID(key)
	if err != nil {
		panic(err)
	}

	return id
}

func ParseLatestPriceHeight(key []byte, height int64) (types.PriceEntryID, error) {
	err := validation.KeyAtLeastLength(key, len(PricesPrefix)+1)
	if err != nil {
		return types.PriceEntryID{}, err
	}

	if !bytes.HasPrefix(key, PricesPrefix) {
		return types.PriceEntryID{}, fmt.Errorf("invalid key prefix. expected 0x%s, actual 0x%s", hex.EncodeToString(PricesPrefix), hex.EncodeToString(key[:2]))
	}

	key = key[len(PricesPrefix):]
	dataLen := int(key[0])
	key = key[1:]

	if err = validation.KeyAtLeastLength(key, dataLen); err != nil {
		return types.PriceEntryID{}, err
	}

	assetDenom := conv.UnsafeBytesToStr(key[:dataLen])

	if err = validation.KeyAtLeastLength(key, 1); err != nil {
		return types.PriceEntryID{}, err
	}

	dataLen = int(key[0])
	key = key[1:]

	if err = validation.KeyLength(key, dataLen); err != nil {
		return types.PriceEntryID{}, err
	}

	baseDenom := conv.UnsafeBytesToStr(key[:dataLen])

	return types.PriceEntryID{
		AssetDenom: assetDenom,
		BaseDenom:  baseDenom,
		Height:     height,
	}, nil
}

func MustParseLatestPriceHeight(key []byte, height int64) types.PriceEntryID {
	id, err := ParseLatestPriceHeight(key, height)
	if err != nil {
		panic(err)
	}

	return id
}
