package keeper

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	vtypes "pkg.akt.dev/go/node/verification/v1"
	"pkg.akt.dev/go/testutil"

	moduletypes "pkg.akt.dev/node/v3/x/verification/types"
)

func TestEscrowBalanceInvariant(t *testing.T) {
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	params := DefaultParams()
	attestation := attestationRecord(provider, auditor)
	escrow := auditEscrowRecord(provider, auditor)
	auditorRecord := auditorRecord(auditor)
	providerBond := providerBondRecord(provider)

	expected := sdk.NewCoins().
		Add(attestation.Fee).
		Add(attestation.Deposit).
		Add(escrow.Fee).
		Add(escrow.ProviderDeposit).
		Add(auditorRecord.BondAmount).
		Add(providerBond.BondedAmount).
		AmountOf(bondDenom)

	bank := &recordingBank{balances: map[string]sdk.Coin{
		bondDenom: sdk.NewCoin(bondDenom, expected),
	}}
	ctx, k := setupStoreKeeperWithOptions(t, WithBankKeeper(bank))
	require.NoError(t, k.SetAttestation(ctx, attestation))
	require.NoError(t, k.SetAuditEscrow(ctx, escrow))
	require.NoError(t, k.SetAuditor(ctx, auditorRecord))
	require.NoError(t, k.SetProviderBond(ctx, providerBond))

	_, broken := EscrowBalanceInvariant(k)(ctx)
	require.False(t, broken)

	escrow.Status = vtypes.AuditEscrowStatusConsumed
	require.NoError(t, k.SetAuditEscrow(ctx, escrow))
	bank.balances[bondDenom] = sdk.NewCoin(bondDenom, expected.Sub(escrow.Fee.Amount))
	_, broken = EscrowBalanceInvariant(k)(ctx)
	require.False(t, broken)

	bank.balances[bondDenom] = params.BondL1
	_, broken = EscrowBalanceInvariant(k)(ctx)
	require.True(t, broken)
}

func TestRegisteredInvariantsCatchInvalidState(t *testing.T) {
	ctx, k := setupStoreKeeper(t)
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)

	expired := attestationRecord(provider, auditor)
	expired.ExpiresAt = ctx.BlockTime()
	require.NoError(t, k.SetAttestation(ctx, expired))

	_, broken := AttestationValidityInvariant(k)(ctx)
	require.True(t, broken)

	expired.Status = vtypes.AttestationStatusExpired
	require.NoError(t, k.SetAttestation(ctx, expired))
	self := attestationRecord(provider, provider)
	require.NoError(t, k.SetAttestation(ctx, self))

	_, broken = SelfAttestationInvariant(k)(ctx)
	require.True(t, broken)
}

func TestProviderBondInvariant(t *testing.T) {
	ctx, k := setupStoreKeeper(t)
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	require.NoError(t, k.SetProviderSnapshot(ctx, providerSnapshotRecord(provider)))
	require.NoError(t, k.SetAttestation(ctx, attestationRecord(provider, auditor)))

	_, broken := ProviderBondInvariant(k)(ctx)
	require.True(t, broken)

	require.NoError(t, k.SetProviderBond(ctx, providerBondRecord(provider)))
	_, broken = ProviderBondInvariant(k)(ctx)
	require.False(t, broken)
}

func TestSnapshotComplianceInvariant(t *testing.T) {
	ctx, k := setupStoreKeeper(t)
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	snapshot := providerSnapshotRecord(provider)
	snapshot.ComplianceDeadline = ctx.BlockTime().Add(-time.Hour)
	require.NoError(t, k.SetProviderSnapshot(ctx, snapshot))
	require.NoError(t, k.SetAttestation(ctx, attestationRecord(provider, auditor)))

	_, broken := SnapshotComplianceInvariant(k)(ctx)
	require.False(t, broken)

	snapshot.Suspended = true
	require.NoError(t, k.SetProviderSnapshot(ctx, snapshot))
	_, broken = SnapshotComplianceInvariant(k)(ctx)
	require.True(t, broken)
}

func TestDiscrepancyConsistencyInvariant(t *testing.T) {
	ctx, k := setupStoreKeeper(t)
	provider := testutil.AccAddress(t)
	auditorA := testutil.AccAddress(t)
	auditorB := testutil.AccAddress(t)

	setupPendingDiscrepancy(t, ctx, k, provider, auditorA, auditorB)
	_, broken := DiscrepancyConsistencyInvariant(k)(ctx)
	require.False(t, broken)

	attestation, found := k.GetAttestation(ctx, provider, auditorA)
	require.True(t, found)
	attestation.DepositStatus = vtypes.DepositStatusEscrowed
	require.NoError(t, k.SetAttestation(ctx, attestation))

	_, broken = DiscrepancyConsistencyInvariant(k)(ctx)
	require.True(t, broken)
}

func TestAuditEscrowInvariant(t *testing.T) {
	ctx, k := setupStoreKeeper(t)
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	attestation := attestationRecord(provider, auditor)
	require.NoError(t, k.SetAttestation(ctx, attestation))

	_, broken := AuditEscrowInvariant(k)(ctx)
	require.True(t, broken)

	escrow := auditEscrowRecord(provider, auditor)
	escrow.ID = attestation.AuditEscrowID
	escrow.Status = vtypes.AuditEscrowStatusConsumed
	require.NoError(t, k.SetAuditEscrow(ctx, escrow))

	_, broken = AuditEscrowInvariant(k)(ctx)
	require.False(t, broken)
}

func TestGraceRecordInvariant(t *testing.T) {
	ctx, k := setupStoreKeeper(t)
	provider := testutil.AccAddress(t)
	require.NoError(t, k.SetProviderVerificationGrace(ctx, graceRecord(provider)))

	_, broken := GraceRecordInvariant(k)(ctx)
	require.True(t, broken)

	discrepancy := discrepancyRecord(provider, testutil.AccAddress(t), testutil.AccAddress(t))
	discrepancy.ID = 11
	discrepancy.ResolutionStatus = vtypes.DiscrepancyStatusPending
	k.SetDiscrepancy(ctx, discrepancy)

	_, broken = GraceRecordInvariant(k)(ctx)
	require.False(t, broken)
}

func TestRegisterInvariants(t *testing.T) {
	registry := &recordingInvariantRegistry{}
	_, k := setupStoreKeeper(t)

	RegisterInvariants(registry, k)

	require.ElementsMatch(t, []string{
		invariantEscrowBalance,
		invariantAttestationValidity,
		invariantFrozenAuditor,
		invariantDiscrepancyConsistency,
		invariantSnapshotCompliance,
		invariantProviderBond,
		invariantSelfAttestation,
		invariantAuditorAuthority,
		invariantAuditEscrow,
		invariantGraceRecord,
	}, registry.routes)
}

type recordingInvariantRegistry struct {
	routes []string
}

func (r *recordingInvariantRegistry) RegisterRoute(moduleName, route string, _ sdk.Invariant) {
	if moduleName == moduletypes.ModuleName {
		r.routes = append(r.routes, route)
	}
}
