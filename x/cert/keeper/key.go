package keeper

import (
	"bytes"
	"math/big"

	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/types/kv"

	types "pkg.akt.dev/go/node/cert/v1"

	"pkg.akt.dev/node/util/validation"
)

const (
	maxSerialLength = 40
)

const (
	CertStateValidPrefixID   = byte(0x01)
	CertStateRevokedPrefixID = byte(0x02)
)

var (
	CertPrefix             = []byte{0x11}
	CertStateValidPrefix   = []byte{CertStateValidPrefixID}
	CertStateRevokedPrefix = []byte{CertStateRevokedPrefixID}
)

func certStateToPrefix(state types.State) []byte {
	var idx []byte

	switch state {
	case types.CertificateValid:
		idx = CertStateValidPrefix
	case types.CertificateRevoked:
		idx = CertStateRevokedPrefix
	default:
		panic("unknown certificate state")
	}

	return idx
}

func buildCertPrefix(state types.State) []byte {
	idx := certStateToPrefix(state)

	res := make([]byte, 0, len(CertPrefix)+len(idx))
	res = append(res, CertPrefix...)
	res = append(res, idx...)

	return res
}

func filterToPrefix(filter types.CertificateFilter) ([]byte, error) {
	prefix := buildCertPrefix(types.State(types.State_value[filter.State]))
	buf := bytes.NewBuffer(prefix)

	if len(filter.Owner) == 0 {
		return buf.Bytes(), nil
	}

	ownerAddr, err := sdk.AccAddressFromBech32(filter.Owner)
	if err != nil {
		return nil, err
	}

	lenPrefixedOwner, err := address.LengthPrefix(ownerAddr)
	if err != nil {
		return nil, err
	}

	if _, err := buf.Write(lenPrefixedOwner); err != nil {
		return nil, err
	}

	if len(filter.Serial) == 0 {
		return buf.Bytes(), nil
	}

	s, _ := big.NewInt(0).SetString(filter.Serial, 10)

	sPrefix, err := serialPrefix(s.Bytes())
	if err != nil {
		return nil, err
	}

	if _, err := buf.Write(sPrefix); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// CertificateKey creates a store key of the format:
// prefix_bytes | state 1 byte | owner_address_len (1 byte) | owner_address_bytes | serial length (1 byte) | serial_bytes
func CertificateKey(state types.State, id types.CertID) ([]byte, error) {
	if id.Owner.Empty() {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, "owner address is empty")
	}

	addr, err := address.LengthPrefix(id.Owner.Bytes())
	if err != nil {
		return nil, err
	}

	serial, err := serialPrefix(id.Serial.Bytes())
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(buildCertPrefix(state))
	if _, err := buf.Write(addr); err != nil {
		return nil, err
	}

	if _, err := buf.Write(serial); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func MustCertificateKey(state types.State, id types.CertID) []byte {
	key, err := CertificateKey(state, id)
	if err != nil {
		panic(err)
	}

	return key
}

// ParseCertKey parse certificate key into id
// format <0x11><state><add len><add bytes><serial length><serial bytes>
func ParseCertKey(from []byte) (types.State, types.CertID, error) {
	res := types.CertID{
		Serial: *big.NewInt(0),
	}

	err := validation.KeyAtLeastLength(from, len(CertPrefix)+1)
	if err != nil {
		return types.CertificateStateInvalid, types.CertID{}, err
	}

	// skip prefix
	from = from[len(CertPrefix):]
	state := types.State(from[0])
	from = from[1:]

	// parse address length
	err = validation.KeyAtLeastLength(from, 1)
	if err != nil {
		return types.CertificateStateInvalid, types.CertID{}, err
	}

	addrLen := from[0]
	from = from[1:]

	// parse address
	err = validation.KeyAtLeastLength(from, int(addrLen))
	if err != nil {
		return types.CertificateStateInvalid, types.CertID{}, err
	}

	addr := from[:addrLen]
	err = sdk.VerifyAddressFormat(addr)
	if err != nil {
		return types.CertificateStateInvalid, types.CertID{}, err
	}

	// parse serial length
	from = from[addrLen:]
	err = validation.KeyAtLeastLength(from, 1)
	if err != nil {
		return types.CertificateStateInvalid, types.CertID{}, err
	}

	serialLen := from[0]

	// parse serial
	from = from[1:]
	err = validation.KeyLength(from, int(serialLen))
	if err != nil {
		return types.CertificateStateInvalid, types.CertID{}, err
	}

	res.Owner = sdk.AccAddress(addr)
	res.Serial.SetBytes(from)

	return state, res, nil
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

	addr := from[:addrLen]
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
		return nil, errorsmod.Wrapf(sdkerrors.ErrUnknownAddress, "serial length should be max %d bytes, got %d", maxSerialLength, bzLen)
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

func ParseCertID(prefix []byte, from []byte) (types.CertID, error) {
	res := types.CertID{
		Serial: *big.NewInt(0),
	}

	// skip prefix if set
	if len(prefix) > 0 {
		from = from[len(prefix):]
	}

	addLen := from[0]

	from = from[1:]

	addr := from[:addLen-1]
	serial := from[addLen:]

	err := sdk.VerifyAddressFormat(addr)
	if err != nil {
		return res, err
	}

	res.Owner = sdk.AccAddress(addr)
	res.Serial.SetBytes(serial)

	return res, nil
}
