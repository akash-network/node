package keeper

import (
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/cert/v1"
)

// Keeper of the provider store
type Keeper interface {
	Querier() types.QueryServer
	Codec() codec.BinaryCodec
	StoreKey() storetypes.StoreKey
	CreateCertificate(sdk.Context, sdk.Address, []byte, []byte) error
	RevokeCertificate(sdk.Context, types.CertID) error
	GetCertificateByID(ctx sdk.Context, id types.CertID) (types.CertificateResponse, bool)
	WithCertificates(ctx sdk.Context, fn func(id types.CertID, certificate types.CertificateResponse) bool)
	WithOwner(ctx sdk.Context, id sdk.Address, fn func(types.CertificateResponse) bool)
	WithOwnerState(ctx sdk.Context, id sdk.Address, state types.State, fn func(types.CertificateResponse) bool)
}

type keeper struct {
	skey storetypes.StoreKey
	cdc  codec.BinaryCodec
}

var _ Keeper = (*keeper)(nil)

// NewKeeper creates and returns an instance for Market keeper
func NewKeeper(cdc codec.BinaryCodec, skey storetypes.StoreKey) Keeper {
	return &keeper{cdc: cdc, skey: skey}
}

// Querier return gRPC query handler
func (k keeper) Querier() types.QueryServer {
	return &querier{keeper: k}
}

// Codec returns keeper codec
func (k keeper) Codec() codec.BinaryCodec {
	return k.cdc
}

// StoreKey returns store key
func (k keeper) StoreKey() storetypes.StoreKey {
	return k.skey
}

func (k keeper) CreateCertificate(ctx sdk.Context, owner sdk.Address, crt []byte, pubkey []byte) error {
	store := ctx.KVStore(k.skey)

	cert, err := types.ParseAndValidateCertificate(owner, crt, pubkey)
	if err != nil {
		return err
	}

	val := types.Certificate{
		State:  types.CertificateValid,
		Cert:   crt,
		Pubkey: pubkey,
	}

	id := types.CertID{
		Owner:  owner,
		Serial: *cert.SerialNumber,
	}
	key := k.findCertificate(ctx, id)
	if len(key) != 0 {
		return types.ErrCertificateExists
	}

	key, err = CertificateKey(val.State, id)
	if err != nil {
		return err
	}

	store.Set(key, k.cdc.MustMarshal(&val))

	return nil
}

func (k keeper) RevokeCertificate(ctx sdk.Context, id types.CertID) error {
	store := ctx.KVStore(k.skey)

	key := k.findCertificate(ctx, id)
	if len(key) == 0 {
		return types.ErrCertificateNotFound
	}

	var cert types.Certificate

	buf := store.Get(key)
	k.cdc.MustUnmarshal(buf, &cert)

	if cert.State == types.CertificateRevoked {
		return types.ErrCertificateAlreadyRevoked
	}

	cert.State = types.CertificateRevoked

	nkey, err := CertificateKey(cert.State, id)
	if err != nil {
		return err
	}

	store.Delete(key)
	store.Set(nkey, k.cdc.MustMarshal(&cert))

	return nil
}

// GetCertificateByID returns a provider with given auditor and owner id
func (k keeper) GetCertificateByID(ctx sdk.Context, id types.CertID) (types.CertificateResponse, bool) {
	store := ctx.KVStore(k.skey)

	key := k.findCertificate(ctx, id)
	if len(key) == 0 {
		return types.CertificateResponse{}, false
	}

	buf := store.Get(key)

	var val types.Certificate
	k.cdc.MustUnmarshal(buf, &val)

	return types.CertificateResponse{
		Certificate: val,
		Serial:      id.Serial.String(),
	}, true
}

// WithCertificates iterates all certificates
func (k keeper) WithCertificates(ctx sdk.Context, fn func(id types.CertID, certificate types.CertificateResponse) bool) {
	store := ctx.KVStore(k.skey)
	iter := storetypes.KVStorePrefixIterator(store, CertPrefix)

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		id, item := k.mustUnmarshal(iter.Key(), iter.Value())
		if stop := fn(id, item); stop {
			break
		}
	}
}

// WithCertificatesState iterates all certificates in certain state
func (k keeper) WithCertificatesState(ctx sdk.Context, state types.State, fn func(certificate types.CertificateResponse) bool) {
	store := ctx.KVStore(k.skey)

	searchPrefix, err := filterToPrefix(types.CertificateFilter{
		State: state.String(),
	})
	if err != nil {
		panic(err)
	}

	iter := storetypes.KVStorePrefixIterator(store, searchPrefix)

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		_, item := k.mustUnmarshal(iter.Key(), iter.Value())
		if stop := fn(item); stop {
			break
		}
	}
}

// WithOwner iterates all certificates by owner
func (k keeper) WithOwner(ctx sdk.Context, id sdk.Address, fn func(types.CertificateResponse) bool) {
	store := ctx.KVStore(k.skey)

	states := []types.State{
		types.CertificateValid,
		types.CertificateRevoked,
	}

	iters := make([]storetypes.Iterator, 0, len(states))
	defer func() {
		for _, iter := range iters {
			_ = iter.Close()
		}
	}()

	for _, state := range states {
		searchPrefix, err := filterToPrefix(types.CertificateFilter{
			Owner: id.String(),
			State: state.String(),
		})
		if err != nil {
			panic(err)
		}

		iter := storetypes.KVStorePrefixIterator(store, searchPrefix)
		iters = append(iters, iter)

		for ; iter.Valid(); iter.Next() {
			_, item := k.mustUnmarshal(iter.Key(), iter.Value())
			if stop := fn(item); stop {
				break
			}
		}
	}
}

// WithOwnerState iterates all certificates by owner in certain state
func (k keeper) WithOwnerState(ctx sdk.Context, id sdk.Address, state types.State, fn func(types.CertificateResponse) bool) {
	store := ctx.KVStore(k.skey)

	searchPrefix, err := filterToPrefix(types.CertificateFilter{
		Owner: id.String(),
		State: state.String(),
	})
	if err != nil {
		panic(err)
	}

	iter := storetypes.KVStorePrefixIterator(store, searchPrefix)
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		_, item := k.mustUnmarshal(iter.Key(), iter.Value())
		if stop := fn(item); stop {
			break
		}
	}
}

func (k keeper) unmarshal(key, val []byte) (types.CertID, types.CertificateResponse, error) {
	_, id, err := ParseCertKey(key)
	if err != nil {
		return types.CertID{}, types.CertificateResponse{}, err
	}

	item := types.CertificateResponse{
		Serial: id.Serial.String(),
	}

	if err := k.cdc.Unmarshal(val, &item.Certificate); err != nil {
		return types.CertID{}, types.CertificateResponse{}, err
	}

	return id, item, nil
}

func (k keeper) mustUnmarshal(key, val []byte) (types.CertID, types.CertificateResponse) {
	id, cert, err := k.unmarshal(key, val)
	if err != nil {
		panic(err)
	}

	return id, cert
}

func (k keeper) findCertificate(ctx sdk.Context, id types.CertID) []byte {
	store := ctx.KVStore(k.skey)

	vKey := MustCertificateKey(types.CertificateValid, id)
	rKey := MustCertificateKey(types.CertificateRevoked, id)

	var key []byte

	if store.Has(vKey) {
		key = vKey
	} else if store.Has(rKey) {
		key = rKey
	}

	return key
}
