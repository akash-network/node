package keeper

import (
	"encoding/binary"
	"time"

	errorsmod "cosmossdk.io/errors"
	storeprefix "cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"

	moduletypes "pkg.akt.dev/node/v3/x/verification/types"
)

type dueQueueEntry struct {
	key     []byte
	dueTime time.Time
}

func (k *keeper) processDueQueue(ctx sdk.Context, queuePrefix byte, blockTime time.Time, limit uint32, process func([]byte, time.Time) error) error {
	if limit == 0 {
		return nil
	}

	store := storeprefix.NewStore(ctx.KVStore(k.skey), singletonKey(queuePrefix))
	iter := store.Iterator(nil, nil)
	defer func() {
		_ = iter.Close()
	}()

	entries := make([]dueQueueEntry, 0, limit)
	for ; iter.Valid() && uint32(len(entries)) < limit; iter.Next() {
		dueTime, err := decodeQueueTime(iter.Key())
		if err != nil {
			return err
		}
		if dueTime.After(blockTime) {
			break
		}

		entries = append(entries, dueQueueEntry{
			key:     append([]byte(nil), iter.Key()...),
			dueTime: dueTime,
		})
	}

	for _, entry := range entries {
		if err := process(entry.key, entry.dueTime); err != nil {
			return err
		}
		store.Delete(entry.key)
	}

	return nil
}

func decodeQueueTime(key []byte) (time.Time, error) {
	if len(key) < 8 {
		return time.Time{}, errorsmod.Wrap(moduletypes.ErrInvalidReason, "malformed verification queue key")
	}
	return time.Unix(0, int64(binary.BigEndian.Uint64(key[:8]))).UTC(), nil
}

func decodeAttestationExpiryQueueKey(key []byte) (sdk.AccAddress, sdk.AccAddress, error) {
	if len(key) < 10 || (len(key)-8)%2 != 0 {
		return nil, nil, errorsmod.Wrap(moduletypes.ErrInvalidReason, "malformed attestation expiry queue key")
	}

	addrLen := (len(key) - 8) / 2
	provider := sdk.AccAddress(append([]byte(nil), key[8:8+addrLen]...))
	auditor := sdk.AccAddress(append([]byte(nil), key[8+addrLen:]...))
	return provider, auditor, nil
}

func decodeAddressQueueKey(key []byte) (sdk.AccAddress, error) {
	if len(key) <= 8 {
		return nil, errorsmod.Wrap(moduletypes.ErrInvalidReason, "malformed address queue key")
	}
	return sdk.AccAddress(append([]byte(nil), key[8:]...)), nil
}

func decodeIDQueueKey(key []byte) (uint64, error) {
	if len(key) != 16 {
		return 0, errorsmod.Wrap(moduletypes.ErrInvalidReason, "malformed id queue key")
	}
	return readID(key[8:]), nil
}
