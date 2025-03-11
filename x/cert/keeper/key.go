package keeper

import (
	"bytes"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/types/kv"

	types "github.com/akash-network/akash-api/go/node/cert/v1beta3"
)

const (
	maxSerialLength = 40
)

// CertificateKey creates a store key of the format:
// prefix_bytes | owner_address_len (1 byte) | owner_address_bytes | serial_bytes
func CertificateKey(id types.CertID) []byte {
	addr, err := address.LengthPrefix(id.Owner.Bytes())
	if err != nil {
		panic(err)
	}

	serial, err := serialPrefix(id.Serial.Bytes())
	if err != nil {
		panic(err)
	}

	buf := bytes.NewBuffer(types.PrefixCertificateID())
	if _, err := buf.Write(addr); err != nil {
		panic(err)
	}

	if _, err := buf.Write(serial); err != nil {
		panic(err)
	}

	return buf.Bytes()
}

<<<<<<< Updated upstream
func CertificatePrefix(id sdk.Address) []byte {
	addr, err := address.LengthPrefix(id.Bytes())
	if err != nil {
		panic(err)
	}

	buf := bytes.NewBuffer(types.PrefixCertificateID())
	if _, err := buf.Write(addr); err != nil {
		panic(err)
	}

	return buf.Bytes()
}

// ParseCertID parse certificate key into id
// format <0x01><add len><add bytes><serial length><serial bytes>

func ParseCertID(prefix []byte, from []byte) (types.CertID, error) {
||||||| Stash base
// ParseCertKey parse certificate key into id
// format <0x01><state><add len><add bytes><serial length><serial bytes>
func ParseCertKey(from []byte) (types.Certificate_State, types.CertID, error) {
=======
// ParseCertKey parse certificate key into id
// format <0x11><state><add len><add bytes><serial length><serial bytes>
func ParseCertKey(from []byte) (types.Certificate_State, types.CertID, error) {
>>>>>>> Stashed changes
	res := types.CertID{
		Serial: *big.NewInt(0),
	}

	kv.AssertKeyAtLeastLength(from, len(prefix))

	// skip prefix if set
	from = from[len(prefix):]

	kv.AssertKeyAtLeastLength(from, 1)

	addrLen := from[0]
	from = from[1:]

	kv.AssertKeyAtLeastLength(from, int(addrLen))

	addr := from[:addrLen]
	err := sdk.VerifyAddressFormat(addr)
	if err != nil {
		return res, err
	}

	from = from[addrLen:]
	kv.AssertKeyAtLeastLength(from, 1)
	serialLen := from[0]

	from = from[1:]
	kv.AssertKeyLength(from, int(serialLen))

	res.Owner = sdk.AccAddress(addr)
	res.Serial.SetBytes(from)

	return res, nil
}

// CertificateKeyLegacy creates a store key of the format:
// prefix_bytes | owner_address_len (1 byte) | owner_address_bytes | serial_bytes
func CertificateKeyLegacy(id types.CertID) []byte {
	buf := bytes.NewBuffer(types.PrefixCertificateID())
	if _, err := buf.Write(address.MustLengthPrefix(id.Owner.Bytes())); err != nil {
		panic(err)
	}

	if _, err := buf.Write(id.Serial.Bytes()); err != nil {
		panic(err)
	}

	return buf.Bytes()
}

func ParseCertIDLegacy(prefix []byte, from []byte) (types.CertID, error) {
	res := types.CertID{
		Serial: *big.NewInt(0),
	}

	kv.AssertKeyAtLeastLength(from, len(prefix))

	// skip prefix if set
	from = from[len(prefix):]

	kv.AssertKeyAtLeastLength(from, 1)

	addrLen := from[0]
	from = from[1:]

	kv.AssertKeyAtLeastLength(from, int(addrLen))

	addr := from[:addrLen-1]
	err := sdk.VerifyAddressFormat(addr)
	if err != nil {
		return res, err
	}

	from = from[addrLen:]

	serial := from

	res.Owner = sdk.AccAddress(addr)
	res.Serial.SetBytes(serial)

	return res, nil
}

func serialPrefix(bz []byte) ([]byte, error) {
	bzLen := len(bz)
	if bzLen == 0 {
		return bz, nil
	}

	if bzLen > maxSerialLength {
		return nil, sdkerrors.Wrapf(sdkerrors.ErrUnknownAddress, "serial length should be max %d bytes, got %d", maxSerialLength, bzLen)
	}

	return append([]byte{byte(bzLen)}, bz...), nil
}

// nolint: unused
func mustSerialPrefix(bz []byte) []byte {
	res, err := serialPrefix(bz)
	if err != nil {
		panic(err)
	}

	return res
}
