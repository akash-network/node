package keeper

import (
	"bytes"

	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	proto "github.com/cosmos/gogoproto/proto"

	vtypes "pkg.akt.dev/go/node/verification/v1"

	moduletypes "pkg.akt.dev/node/v2/x/verification/types"
)

const (
	auditEscrowRecordTypeURL = "/akash.verification.v1.AuditEscrowRecord"
	graceRecordTypeURL       = "/akash.verification.v1.ProviderVerificationGraceRecord"
)

func (k *keeper) GetParams(ctx sdk.Context) vtypes.Params {
	store := ctx.KVStore(k.skey)
	bz := store.Get(singletonKey(keyParams))
	if bz == nil {
		return DefaultParams()
	}

	var params vtypes.Params
	k.cdc.MustUnmarshal(bz, &params)
	return params
}

func (k *keeper) SetParams(ctx sdk.Context, params vtypes.Params) {
	ctx.KVStore(k.skey).Set(singletonKey(keyParams), k.cdc.MustMarshal(&params))
}

func (k *keeper) GetAuditor(ctx sdk.Context, auditor sdk.AccAddress) (vtypes.AuditorRecord, bool) {
	store := ctx.KVStore(k.skey)
	bz := store.Get(addressKey(prefixAuditor, auditor))
	if bz == nil {
		return vtypes.AuditorRecord{}, false
	}

	var record vtypes.AuditorRecord
	k.cdc.MustUnmarshal(bz, &record)
	return record, true
}

func (k *keeper) SetAuditor(ctx sdk.Context, record vtypes.AuditorRecord) error {
	auditor, err := sdk.AccAddressFromBech32(record.Address)
	if err != nil {
		return err
	}

	ctx.KVStore(k.skey).Set(addressKey(prefixAuditor, auditor), k.cdc.MustMarshal(&record))
	return nil
}

func (k *keeper) WithAuditors(ctx sdk.Context, fn func(vtypes.AuditorRecord) bool) {
	store := prefix.NewStore(ctx.KVStore(k.skey), singletonKey(prefixAuditor))
	iter := store.Iterator(nil, nil)
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var record vtypes.AuditorRecord
		k.cdc.MustUnmarshal(iter.Value(), &record)
		if stop := fn(record); stop {
			break
		}
	}
}

func (k *keeper) GetAttestation(ctx sdk.Context, provider, auditor sdk.AccAddress) (vtypes.AttestationRecord, bool) {
	store := ctx.KVStore(k.skey)
	bz := store.Get(attestationKey(provider, auditor))
	if bz == nil {
		return vtypes.AttestationRecord{}, false
	}

	var record vtypes.AttestationRecord
	k.cdc.MustUnmarshal(bz, &record)
	return record, true
}

func (k *keeper) SetAttestation(ctx sdk.Context, record vtypes.AttestationRecord) error {
	provider, err := sdk.AccAddressFromBech32(record.Provider)
	if err != nil {
		return err
	}
	auditor, err := sdk.AccAddressFromBech32(record.Auditor)
	if err != nil {
		return err
	}

	store := ctx.KVStore(k.skey)
	store.Set(attestationKey(provider, auditor), k.cdc.MustMarshal(&record))
	if record.Status == vtypes.AttestationStatusValid {
		store.Set(auditorAttestationKey(auditor, provider), []byte{})
		store.Set(attestationExpiryQueueKey(record.ExpiresAt, provider, auditor), []byte{})
	} else {
		store.Delete(auditorAttestationKey(auditor, provider))
		store.Delete(attestationExpiryQueueKey(record.ExpiresAt, provider, auditor))
	}
	return nil
}

func (k *keeper) WithAttestations(ctx sdk.Context, fn func(vtypes.AttestationRecord) bool) {
	store := prefix.NewStore(ctx.KVStore(k.skey), singletonKey(prefixAttestation))
	iter := store.Iterator(nil, nil)
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var record vtypes.AttestationRecord
		k.cdc.MustUnmarshal(iter.Value(), &record)
		if stop := fn(record); stop {
			break
		}
	}
}

func (k *keeper) WithProviderAttestations(ctx sdk.Context, provider sdk.AccAddress, status vtypes.AttestationStatus, fn func(vtypes.AttestationRecord) bool) {
	store := prefix.NewStore(ctx.KVStore(k.skey), addressKey(prefixAttestation, provider))
	iter := store.Iterator(nil, nil)
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var record vtypes.AttestationRecord
		k.cdc.MustUnmarshal(iter.Value(), &record)
		if status != vtypes.AttestationStatusUnspecified && record.Status != status {
			continue
		}
		if stop := fn(record); stop {
			break
		}
	}
}

func (k *keeper) WithAuditorAttestations(ctx sdk.Context, auditor sdk.AccAddress, fn func(vtypes.AttestationRecord) bool) {
	store := prefix.NewStore(ctx.KVStore(k.skey), auditorAttestationPrefix(auditor))
	iter := store.Iterator(nil, nil)
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		record, found := k.GetAttestation(ctx, sdk.AccAddress(iter.Key()), auditor)
		if !found {
			continue
		}
		if stop := fn(record); stop {
			break
		}
	}
}

func (k *keeper) GetDiscrepancy(ctx sdk.Context, id uint64) (vtypes.DiscrepancyEvent, bool) {
	store := ctx.KVStore(k.skey)
	bz := store.Get(idKey(prefixDiscrepancy, id))
	if bz == nil {
		return vtypes.DiscrepancyEvent{}, false
	}

	var record vtypes.DiscrepancyEvent
	k.cdc.MustUnmarshal(bz, &record)
	return record, true
}

func (k *keeper) SetDiscrepancy(ctx sdk.Context, record vtypes.DiscrepancyEvent) {
	store := ctx.KVStore(k.skey)
	store.Set(idKey(prefixDiscrepancy, record.ID), k.cdc.MustMarshal(&record))

	timeout := record.Timestamp.Add(k.GetParams(ctx).DiscrepancyResolutionTimeout)
	if record.ResolutionStatus == vtypes.DiscrepancyStatusPending {
		store.Set(discrepancyTimeoutQueueKey(timeout, record.ID), []byte{})
	} else {
		store.Delete(discrepancyTimeoutQueueKey(timeout, record.ID))
	}
}

func (k *keeper) WithDiscrepancies(ctx sdk.Context, status vtypes.DiscrepancyStatus, fn func(vtypes.DiscrepancyEvent) bool) {
	store := prefix.NewStore(ctx.KVStore(k.skey), singletonKey(prefixDiscrepancy))
	iter := store.Iterator(nil, nil)
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var record vtypes.DiscrepancyEvent
		k.cdc.MustUnmarshal(iter.Value(), &record)
		if status != vtypes.DiscrepancyStatusUnspecified && record.ResolutionStatus != status {
			continue
		}
		if stop := fn(record); stop {
			break
		}
	}
}

func (k *keeper) GetProviderBond(ctx sdk.Context, provider sdk.AccAddress) (vtypes.ProviderBondRecord, bool) {
	store := ctx.KVStore(k.skey)
	bz := store.Get(addressKey(prefixProviderBond, provider))
	if bz == nil {
		return vtypes.ProviderBondRecord{}, false
	}

	var record vtypes.ProviderBondRecord
	k.cdc.MustUnmarshal(bz, &record)
	return record, true
}

func (k *keeper) SetProviderBond(ctx sdk.Context, record vtypes.ProviderBondRecord) error {
	provider, err := sdk.AccAddressFromBech32(record.Provider)
	if err != nil {
		return err
	}

	ctx.KVStore(k.skey).Set(addressKey(prefixProviderBond, provider), k.cdc.MustMarshal(&record))
	return nil
}

func (k *keeper) WithProviderBonds(ctx sdk.Context, fn func(vtypes.ProviderBondRecord) bool) {
	store := prefix.NewStore(ctx.KVStore(k.skey), singletonKey(prefixProviderBond))
	iter := store.Iterator(nil, nil)
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var record vtypes.ProviderBondRecord
		k.cdc.MustUnmarshal(iter.Value(), &record)
		if stop := fn(record); stop {
			break
		}
	}
}

func (k *keeper) GetProviderSnapshot(ctx sdk.Context, provider sdk.AccAddress) (vtypes.ProviderSnapshotRecord, bool) {
	store := ctx.KVStore(k.skey)
	bz := store.Get(addressKey(prefixProviderSnapshot, provider))
	if bz == nil {
		return vtypes.ProviderSnapshotRecord{}, false
	}

	var record vtypes.ProviderSnapshotRecord
	k.cdc.MustUnmarshal(bz, &record)
	return record, true
}

func (k *keeper) SetProviderSnapshot(ctx sdk.Context, record vtypes.ProviderSnapshotRecord) error {
	provider, err := sdk.AccAddressFromBech32(record.Provider)
	if err != nil {
		return err
	}

	store := ctx.KVStore(k.skey)
	store.Set(addressKey(prefixProviderSnapshot, provider), k.cdc.MustMarshal(&record))
	if record.Suspended {
		store.Delete(snapshotComplianceQueueKey(record.ComplianceDeadline, provider))
	} else {
		store.Set(snapshotComplianceQueueKey(record.ComplianceDeadline, provider), []byte{})
	}
	return nil
}

func (k *keeper) WithProviderSnapshots(ctx sdk.Context, fn func(vtypes.ProviderSnapshotRecord) bool) {
	store := prefix.NewStore(ctx.KVStore(k.skey), singletonKey(prefixProviderSnapshot))
	iter := store.Iterator(nil, nil)
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var record vtypes.ProviderSnapshotRecord
		k.cdc.MustUnmarshal(iter.Value(), &record)
		if stop := fn(record); stop {
			break
		}
	}
}

func (k *keeper) GetAuditEscrow(ctx sdk.Context, id uint64) (vtypes.AuditEscrowRecord, bool) {
	store := ctx.KVStore(k.skey)
	bz := store.Get(idKey(prefixAuditEscrow, id))
	if bz == nil {
		return vtypes.AuditEscrowRecord{}, false
	}

	var record vtypes.AuditEscrowRecord
	if err := k.unmarshalStoreRecord(bz, auditEscrowRecordTypeURL, &record); err != nil {
		panic(err)
	}
	return record, true
}

func (k *keeper) SetAuditEscrow(ctx sdk.Context, record vtypes.AuditEscrowRecord) error {
	provider, err := sdk.AccAddressFromBech32(record.Provider)
	if err != nil {
		return err
	}

	store := ctx.KVStore(k.skey)
	store.Set(idKey(prefixAuditEscrow, record.ID), k.marshalStoreRecord(auditEscrowRecordTypeURL, &record))
	store.Set(providerAuditEscrowKey(provider, record.ID), []byte{})
	if record.Status == vtypes.AuditEscrowStatusOpen {
		store.Set(auditEscrowExpiryQueueKey(record.ExpiresAt, record.ID), []byte{})
	} else {
		store.Delete(auditEscrowExpiryQueueKey(record.ExpiresAt, record.ID))
	}

	if record.ConsumedByAuditor != "" {
		auditor, err := sdk.AccAddressFromBech32(record.ConsumedByAuditor)
		if err != nil {
			return err
		}
		store.Set(auditorAuditEscrowKey(auditor, record.ID), []byte{})
	}
	return nil
}

func (k *keeper) WithAuditEscrows(ctx sdk.Context, fn func(vtypes.AuditEscrowRecord) bool) {
	store := prefix.NewStore(ctx.KVStore(k.skey), singletonKey(prefixAuditEscrow))
	iter := store.Iterator(nil, nil)
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var record vtypes.AuditEscrowRecord
		if err := k.unmarshalStoreRecord(iter.Value(), auditEscrowRecordTypeURL, &record); err != nil {
			panic(err)
		}
		if stop := fn(record); stop {
			break
		}
	}
}

func (k *keeper) WithProviderAuditEscrows(ctx sdk.Context, provider sdk.AccAddress, status vtypes.AuditEscrowStatus, fn func(vtypes.AuditEscrowRecord) bool) {
	store := prefix.NewStore(ctx.KVStore(k.skey), providerAuditEscrowPrefix(provider))
	iter := store.Iterator(nil, nil)
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		id := readID(iter.Key())
		record, found := k.GetAuditEscrow(ctx, id)
		if !found {
			continue
		}
		if status != vtypes.AuditEscrowStatusUnspecified && record.Status != status {
			continue
		}
		if stop := fn(record); stop {
			break
		}
	}
}

func (k *keeper) GetProviderVerificationGrace(ctx sdk.Context, provider sdk.AccAddress) (vtypes.ProviderVerificationGraceRecord, bool) {
	var result vtypes.ProviderVerificationGraceRecord
	var found bool
	k.WithProviderVerificationGraces(ctx, provider, func(record vtypes.ProviderVerificationGraceRecord) bool {
		result = record
		found = true
		return true
	})
	return result, found
}

func (k *keeper) SetProviderVerificationGrace(ctx sdk.Context, record vtypes.ProviderVerificationGraceRecord) error {
	provider, err := sdk.AccAddressFromBech32(record.Provider)
	if err != nil {
		return err
	}

	store := ctx.KVStore(k.skey)
	store.Set(idKey(prefixGraceRecord, record.ID), k.marshalStoreRecord(graceRecordTypeURL, &record))
	store.Set(providerGraceKey(provider, record.ID), []byte{})
	if record.Status == vtypes.VerificationGraceStatusActive {
		store.Set(graceExpiryQueueKey(record.ExpiresAt, record.ID), []byte{})
	} else {
		store.Delete(graceExpiryQueueKey(record.ExpiresAt, record.ID))
	}
	return nil
}

func (k *keeper) WithProviderVerificationGraces(ctx sdk.Context, provider sdk.AccAddress, fn func(vtypes.ProviderVerificationGraceRecord) bool) {
	store := prefix.NewStore(ctx.KVStore(k.skey), providerGracePrefix(provider))
	iter := store.Iterator(nil, nil)
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		id := readID(iter.Key())
		record, found := k.getProviderVerificationGraceByID(ctx, id)
		if !found {
			continue
		}
		if stop := fn(record); stop {
			break
		}
	}
}

func (k *keeper) WithVerificationGraces(ctx sdk.Context, fn func(vtypes.ProviderVerificationGraceRecord) bool) {
	store := prefix.NewStore(ctx.KVStore(k.skey), singletonKey(prefixGraceRecord))
	iter := store.Iterator(nil, nil)
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var record vtypes.ProviderVerificationGraceRecord
		if err := k.unmarshalStoreRecord(iter.Value(), graceRecordTypeURL, &record); err != nil {
			panic(err)
		}
		if stop := fn(record); stop {
			break
		}
	}
}

func (k *keeper) getProviderVerificationGraceByID(ctx sdk.Context, id uint64) (vtypes.ProviderVerificationGraceRecord, bool) {
	store := ctx.KVStore(k.skey)
	bz := store.Get(idKey(prefixGraceRecord, id))
	if bz == nil {
		return vtypes.ProviderVerificationGraceRecord{}, false
	}

	var record vtypes.ProviderVerificationGraceRecord
	if err := k.unmarshalStoreRecord(bz, graceRecordTypeURL, &record); err != nil {
		panic(err)
	}
	return record, true
}

func (k *keeper) GetNextDiscrepancyID(ctx sdk.Context) uint64 {
	return k.getNextID(ctx, keyNextDiscrepancyID)
}

func (k *keeper) SetNextDiscrepancyID(ctx sdk.Context, id uint64) {
	k.setNextID(ctx, keyNextDiscrepancyID, id)
}

func (k *keeper) NextDiscrepancyID(ctx sdk.Context) uint64 {
	id := k.GetNextDiscrepancyID(ctx)
	k.SetNextDiscrepancyID(ctx, id+1)
	return id
}

func (k *keeper) GetNextAuditEscrowID(ctx sdk.Context) uint64 {
	return k.getNextID(ctx, keyNextAuditEscrowID)
}

func (k *keeper) SetNextAuditEscrowID(ctx sdk.Context, id uint64) {
	k.setNextID(ctx, keyNextAuditEscrowID, id)
}

func (k *keeper) NextAuditEscrowID(ctx sdk.Context) uint64 {
	id := k.GetNextAuditEscrowID(ctx)
	k.SetNextAuditEscrowID(ctx, id+1)
	return id
}

func (k *keeper) GetNextGraceRecordID(ctx sdk.Context) uint64 {
	return k.getNextID(ctx, keyNextGraceRecordID)
}

func (k *keeper) SetNextGraceRecordID(ctx sdk.Context, id uint64) {
	k.setNextID(ctx, keyNextGraceRecordID, id)
}

func (k *keeper) NextGraceRecordID(ctx sdk.Context) uint64 {
	id := k.GetNextGraceRecordID(ctx)
	k.SetNextGraceRecordID(ctx, id+1)
	return id
}

func (k *keeper) getNextID(ctx sdk.Context, key byte) uint64 {
	bz := ctx.KVStore(k.skey).Get(singletonKey(key))
	if bz == nil {
		return 1
	}
	return readID(bz)
}

func (k *keeper) setNextID(ctx sdk.Context, key byte, id uint64) {
	ctx.KVStore(k.skey).Set(singletonKey(key), encodeID(id))
}

func (k *keeper) marshalStoreRecord(typeURL string, value proto.Message) []byte {
	record := vtypes.VerificationStoreRecord{
		TypeURL: typeURL,
		Value:   k.cdc.MustMarshal(value),
	}
	return k.cdc.MustMarshal(&record)
}

func (k *keeper) unmarshalStoreRecord(bz []byte, typeURL string, value proto.Message) error {
	var record vtypes.VerificationStoreRecord
	k.cdc.MustUnmarshal(bz, &record)
	if !bytes.Equal([]byte(record.TypeURL), []byte(typeURL)) {
		return moduletypes.ErrUnknownVerificationRecordType.Wrapf("expected %s, got %s", typeURL, record.TypeURL)
	}

	k.cdc.MustUnmarshal(record.Value, value)
	return nil
}
