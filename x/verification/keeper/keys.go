package keeper

import (
	"encoding/binary"

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

func readID(bz []byte) uint64 {
	return binary.BigEndian.Uint64(bz)
}

func encodeID(id uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, id)
	return buf
}
