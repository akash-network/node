package keeper

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"math/big"

	"github.com/ovrclk/akash/x/cert/types"
)

// Keeper of the provider store
type Keeper interface {
	Querier() types.QueryServer
	Codec() codec.BinaryMarshaler
	CreateCertificate(sdk.Context, sdk.Address, []byte, []byte) error
	RevokeCertificate(sdk.Context, types.CertID) error
	GetCertificateByID(ctx sdk.Context, id types.CertID) (types.CertificateResponse, bool)
	WithCertificates(ctx sdk.Context, fn func(certificate types.CertificateResponse) bool)
	WithCertificatesState(ctx sdk.Context, state types.Certificate_State, fn func(certificate types.CertificateResponse) bool)
	WithOwner(ctx sdk.Context, id sdk.Address, fn func(types.CertificateResponse) bool)
	WithOwnerState(ctx sdk.Context, id sdk.Address, state types.Certificate_State, fn func(types.CertificateResponse) bool)

	// Used for upgrade
	IterateCertificatesRaw(ctx sdk.Context, fn func(owner sdk.Address, serial big.Int, certificateRaw []byte) bool)
	SetCertificate(ctx sdk.Context, owner sdk.Address, serial big.Int, crt types.Certificate)
}

type keeper struct {
	skey sdk.StoreKey
	cdc  codec.BinaryMarshaler
}

var _ Keeper = (*keeper)(nil)

// NewKeeper creates and returns an instance for Market keeper
func NewKeeper(cdc codec.BinaryMarshaler, skey sdk.StoreKey) Keeper {
	return &keeper{cdc: cdc, skey: skey}
}

// Querier return gRPC query handler
func (k keeper) Querier() types.QueryServer {
	return &querier{keeper: k}
}

// Codec returns keeper codec
func (k keeper) Codec() codec.BinaryMarshaler {
	return k.cdc
}

func (k keeper) SetCertificate(ctx sdk.Context, owner sdk.Address, serial big.Int, crt types.Certificate) {
	store := ctx.KVStore(k.skey)

	key := certificateKey(types.CertID{
		Owner:  owner,
		Serial: serial,
	})

	store.Set(key, k.cdc.MustMarshalBinaryBare(&crt))
}

func (k keeper) CreateCertificate(ctx sdk.Context, owner sdk.Address, crt []byte, pubkey []byte) error {
	store := ctx.KVStore(k.skey)

	cert, err := types.ParseAndValidateCertificate(owner, crt, pubkey)
	if err != nil {
		return err
	}

	key := certificateKey(types.CertID{
		Owner:  owner,
		Serial: *cert.SerialNumber,
	})

	if store.Has(key) {
		return types.ErrCertificateExists
	}

	iter := sdk.KVStorePrefixIterator(store, certificatePrefix(owner))
	defer func() {
		_ = iter.Close()
	}()

	val := types.Certificate{
		State:  types.CertificateValid,
		Cert:   crt,
		Pubkey: pubkey,
	}

	store.Set(key, k.cdc.MustMarshalBinaryBare(&val))

	return nil
}

func (k keeper) RevokeCertificate(ctx sdk.Context, id types.CertID) error {
	store := ctx.KVStore(k.skey)
	key := certificateKey(id)

	buf := store.Get(key)
	if buf == nil {
		return types.ErrCertificateNotFound
	}

	var cert types.Certificate
	k.cdc.MustUnmarshalBinaryBare(buf, &cert)

	if cert.State == types.CertificateRevoked {
		return types.ErrCertificateAlreadyRevoked
	}

	cert.State = types.CertificateRevoked

	store.Set(key, k.cdc.MustMarshalBinaryBare(&cert))

	return nil
}

// GetCertificateByID returns a provider with given auditor and owner id
func (k keeper) GetCertificateByID(ctx sdk.Context, id types.CertID) (types.CertificateResponse, bool) {
	store := ctx.KVStore(k.skey)

	buf := store.Get(certificateKey(id))
	if buf == nil {
		return types.CertificateResponse{}, false
	}

	var val types.Certificate
	k.cdc.MustUnmarshalBinaryBare(buf, &val)

	return types.CertificateResponse{
		Certificate: val,
		Serial:      id.Serial.String(),
	}, true
}

func (k keeper) IterateCertificatesRaw(ctx sdk.Context, fn func(owner sdk.Address, serial big.Int, certificateRaw []byte) bool) {
	store := ctx.KVStore(k.skey)
	iter := store.Iterator(nil, nil)

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		owner := certificateOwnerFromKey(iter.Key())
		serial := certificateSerialFromKey(iter.Key())
		if stop := fn(owner, serial, iter.Value()); stop {
			break
		}
	}
}

// WithCertificates iterates all certificates
func (k keeper) WithCertificates(ctx sdk.Context, fn func(certificate types.CertificateResponse) bool) {
	store := ctx.KVStore(k.skey)
	iter := store.Iterator(nil, nil)

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		item := k.mustUnmarshal(iter.Key(), iter.Value())
		if stop := fn(item); stop {
			break
		}
	}
}

// WithCertificatesState iterates all certificates in certain state
func (k keeper) WithCertificatesState(ctx sdk.Context, state types.Certificate_State, fn func(certificate types.CertificateResponse) bool) {
	store := ctx.KVStore(k.skey)
	iter := store.Iterator(nil, nil)

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		item := k.mustUnmarshal(iter.Key(), iter.Value())
		if item.Certificate.State == state {
			if stop := fn(item); stop {
				break
			}
		}
	}
}

// WithOwner iterates all certificates by owner
func (k keeper) WithOwner(ctx sdk.Context, id sdk.Address, fn func(types.CertificateResponse) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, certificatePrefix(id))
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		item := k.mustUnmarshal(iter.Key(), iter.Value())
		if stop := fn(item); stop {
			break
		}
	}
}

// WithOwnerState iterates all certificates by owner in certain state
func (k keeper) WithOwnerState(ctx sdk.Context, id sdk.Address, state types.Certificate_State, fn func(types.CertificateResponse) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, certificatePrefix(id))
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		item := k.mustUnmarshal(iter.Key(), iter.Value())
		if item.Certificate.State == state {
			if stop := fn(item); stop {
				break
			}
		}
	}
}

func (k keeper) mustUnmarshal(key, val []byte) types.CertificateResponse {
	serial := certificateSerialFromKey(key)
	item := types.CertificateResponse{
		Serial: serial.String(),
	}
	k.cdc.MustUnmarshalBinaryBare(val, &item.Certificate)

	return item
}

func (k keeper) unmarshalIterator(key, val []byte) (types.CertificateResponse, error) {
	serial := certificateSerialFromKey(key)
	item := types.CertificateResponse{
		Serial: serial.String(),
	}

	if err := k.cdc.UnmarshalBinaryBare(val, &item.Certificate); err != nil {
		return types.CertificateResponse{}, err
	}

	return item, nil
}
