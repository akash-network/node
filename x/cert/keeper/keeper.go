package keeper

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/x/cert/types"
)

// Keeper of the provider store
type Keeper struct {
	skey sdk.StoreKey
	cdc  codec.BinaryMarshaler
}

// NewKeeper creates and returns an instance for Market keeper
func NewKeeper(cdc codec.BinaryMarshaler, skey sdk.StoreKey) Keeper {
	return Keeper{cdc: cdc, skey: skey}
}

// Codec returns keeper codec
func (k Keeper) Codec() codec.BinaryMarshaler {
	return k.cdc
}

func (k Keeper) CreateCertificate(ctx sdk.Context, owner sdk.Address, crt []byte, pubkey []byte) error {
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

func (k Keeper) RevokeCertificate(ctx sdk.Context, id types.CertID) error {
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
func (k Keeper) GetCertificateByID(ctx sdk.Context, id types.CertID) (types.Certificate, bool) {
	store := ctx.KVStore(k.skey)

	buf := store.Get(certificateKey(id))
	if buf == nil {
		return types.Certificate{}, false
	}

	var val types.Certificate
	k.cdc.MustUnmarshalBinaryBare(buf, &val)

	return val, true
}

// WithCertificates iterates all certificates
func (k Keeper) WithCertificates(ctx sdk.Context, fn func(certificate types.Certificate) bool) {
	store := ctx.KVStore(k.skey)
	iter := store.Iterator(nil, nil)

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val types.Certificate
		k.cdc.MustUnmarshalBinaryBare(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

// WithCertificates iterates all certificates
func (k Keeper) WithCertificatesState(ctx sdk.Context, state types.Certificate_State, fn func(certificate types.Certificate) bool) {
	store := ctx.KVStore(k.skey)
	iter := store.Iterator(nil, nil)

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val types.Certificate
		k.cdc.MustUnmarshalBinaryBare(iter.Value(), &val)
		if val.State == state {
			if stop := fn(val); stop {
				break
			}
		}
	}
}

// WithOwner iterates all certificates by owner
func (k Keeper) WithOwner(ctx sdk.Context, id sdk.Address, fn func(types.Certificate) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, certificatePrefix(id))
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val types.Certificate
		k.cdc.MustUnmarshalBinaryBare(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

// WithOwner iterates all certificates by owner
func (k Keeper) WithOwnerState(ctx sdk.Context, id sdk.Address, state types.Certificate_State, fn func(types.Certificate) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, certificatePrefix(id))
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val types.Certificate
		k.cdc.MustUnmarshalBinaryBare(iter.Value(), &val)
		if val.State == state {
			if stop := fn(val); stop {
				break
			}
		}
	}
}
