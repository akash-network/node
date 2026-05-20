package keeper

import (
	"encoding/binary"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	prefixAuditor byte = iota + 1
	prefixAttestation
	prefixDiscrepancy
	prefixProviderBond
	prefixProviderSnapshot
	keyParams
	keyNextDiscrepancyID
	prefixAuditEscrow
	keyNextAuditEscrowID
	prefixGraceRecord
	keyNextGraceRecordID
)

const (
	prefixAuditorAttestation byte = 0x10 + iota
	prefixProviderAuditEscrow
	prefixAuditorAuditEscrow
	prefixProviderGrace
)

const (
	prefixQueueAttestationExpiry     byte = 0x20
	prefixQueueAuditorRenewal        byte = 0x21
	prefixQueueSnapshotCompliance    byte = 0x22
	prefixQueueProviderBondUnbonding byte = 0x23
	prefixQueueAuditorBondUnbonding  byte = 0x24
	prefixQueueDiscrepancyTimeout    byte = 0x25
	prefixQueueAuditEscrowExpiry     byte = 0x26
	prefixQueueGraceExpiry           byte = 0x27
)

func singletonKey(prefix byte) []byte {
	return []byte{prefix}
}

func addressKey(prefix byte, addr sdk.AccAddress) []byte {
	key := make([]byte, 1+len(addr))
	key[0] = prefix
	copy(key[1:], addr)
	return key
}

func idKey(prefix byte, id uint64) []byte {
	key := make([]byte, 9)
	key[0] = prefix
	binary.BigEndian.PutUint64(key[1:], id)
	return key
}

func attestationKey(provider, auditor sdk.AccAddress) []byte {
	key := make([]byte, 1+len(provider)+len(auditor))
	key[0] = prefixAttestation
	copy(key[1:], provider)
	copy(key[1+len(provider):], auditor)
	return key
}

func auditorAttestationKey(auditor, provider sdk.AccAddress) []byte {
	key := make([]byte, 1+len(auditor)+len(provider))
	key[0] = prefixAuditorAttestation
	copy(key[1:], auditor)
	copy(key[1+len(auditor):], provider)
	return key
}

func auditorAttestationPrefix(auditor sdk.AccAddress) []byte {
	return addressKey(prefixAuditorAttestation, auditor)
}

func providerAuditEscrowKey(provider sdk.AccAddress, id uint64) []byte {
	key := make([]byte, 1+len(provider)+8)
	key[0] = prefixProviderAuditEscrow
	copy(key[1:], provider)
	binary.BigEndian.PutUint64(key[1+len(provider):], id)
	return key
}

func providerAuditEscrowPrefix(provider sdk.AccAddress) []byte {
	return addressKey(prefixProviderAuditEscrow, provider)
}

func auditorAuditEscrowKey(auditor sdk.AccAddress, id uint64) []byte {
	key := make([]byte, 1+len(auditor)+8)
	key[0] = prefixAuditorAuditEscrow
	copy(key[1:], auditor)
	binary.BigEndian.PutUint64(key[1+len(auditor):], id)
	return key
}

func providerGraceKey(provider sdk.AccAddress, id uint64) []byte {
	key := make([]byte, 1+len(provider)+8)
	key[0] = prefixProviderGrace
	copy(key[1:], provider)
	binary.BigEndian.PutUint64(key[1+len(provider):], id)
	return key
}

func providerGracePrefix(provider sdk.AccAddress) []byte {
	return addressKey(prefixProviderGrace, provider)
}

func attestationExpiryQueueKey(expiresAt time.Time, provider, auditor sdk.AccAddress) []byte {
	return timeQueueKey(prefixQueueAttestationExpiry, expiresAt, provider, auditor)
}

func snapshotComplianceQueueKey(deadline time.Time, provider sdk.AccAddress) []byte {
	return timeQueueKey(prefixQueueSnapshotCompliance, deadline, provider)
}

func auditEscrowExpiryQueueKey(expiresAt time.Time, id uint64) []byte {
	return timeQueueKey(prefixQueueAuditEscrowExpiry, expiresAt, encodeID(id))
}

func graceExpiryQueueKey(expiresAt time.Time, id uint64) []byte {
	return timeQueueKey(prefixQueueGraceExpiry, expiresAt, encodeID(id))
}

func timeQueueKey(prefix byte, at time.Time, parts ...[]byte) []byte {
	size := 9
	for _, part := range parts {
		size += len(part)
	}

	key := make([]byte, size)
	key[0] = prefix
	binary.BigEndian.PutUint64(key[1:], uint64(at.UnixNano()))

	offset := 9
	for _, part := range parts {
		copy(key[offset:], part)
		offset += len(part)
	}
	return key
}

func readID(bz []byte) uint64 {
	return binary.BigEndian.Uint64(bz)
}

func encodeID(id uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, id)
	return buf
}
