package keeper

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	vtypes "pkg.akt.dev/go/node/verification/v1"
	"pkg.akt.dev/go/testutil"

	moduletypes "pkg.akt.dev/node/v2/x/verification/types"
)

func TestEndBlockerExpiresAttestation(t *testing.T) {
	bank := &recordingBank{}
	ctx, k := setupStoreKeeperWithOptions(t, WithBankKeeper(bank))
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	record := attestationRecord(provider, auditor)
	record.ExpiresAt = ctx.BlockTime().Add(-time.Second)

	require.NoError(t, k.SetAttestation(ctx, record))
	require.NoError(t, k.EndBlocker(ctx))

	got, found := k.GetAttestation(ctx, provider, auditor)
	require.True(t, found)
	require.Equal(t, vtypes.AttestationStatusExpired, got.Status)
	require.Equal(t, vtypes.FeeStatusReleasedToAuditor, got.FeeStatus)
	require.Equal(t, vtypes.DepositStatusReturnedToAuditor, got.DepositStatus)
	require.Equal(t, vtypes.FaultAttributionNoFault, got.FaultAttribution)

	var byAuditor []vtypes.AttestationRecord
	k.WithAuditorAttestations(ctx, auditor, func(record vtypes.AttestationRecord) bool {
		byAuditor = append(byAuditor, record)
		return false
	})
	require.Empty(t, byAuditor)
	require.Equal(t, []bankTransfer{
		{to: auditor, module: moduletypes.ModuleName, amt: sdk.NewCoins(record.Fee)},
		{to: auditor, module: moduletypes.ModuleName, amt: sdk.NewCoins(record.Deposit)},
	}, bank.moduleToAccount)
}

func TestEndBlockerIgnoresStaleAttestationExpiryQueue(t *testing.T) {
	bank := &recordingBank{}
	ctx, k := setupStoreKeeperWithOptions(t, WithBankKeeper(bank))
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	record := attestationRecord(provider, auditor)
	record.ExpiresAt = ctx.BlockTime().Add(-time.Second)
	require.NoError(t, k.SetAttestation(ctx, record))

	record.ExpiresAt = ctx.BlockTime().Add(time.Hour)
	require.NoError(t, k.SetAttestation(ctx, record))
	require.NoError(t, k.EndBlocker(ctx))

	got, found := k.GetAttestation(ctx, provider, auditor)
	require.True(t, found)
	require.Equal(t, vtypes.AttestationStatusValid, got.Status)
	require.Equal(t, record.ExpiresAt, got.ExpiresAt)
	require.Empty(t, bank.moduleToAccount)
}

func TestEndBlockerLapsesAuditorAtRenewalDeadline(t *testing.T) {
	ctx, k := setupStoreKeeper(t)
	auditor := testutil.AccAddress(t)
	record := auditorRecord(auditor)
	record.RenewalDeadline = ctx.BlockTime().Add(-time.Second)

	require.NoError(t, k.SetAuditor(ctx, record))
	require.NoError(t, k.EndBlocker(ctx))

	got, found := k.GetAuditor(ctx, auditor)
	require.True(t, found)
	require.Equal(t, vtypes.AuditorStatusLapsed, got.Status)
	require.Equal(t, record.RenewalDeadline, got.RenewalDeadline)
}

func TestEndBlockerCompletesAuditorBondUnbonding(t *testing.T) {
	bank := &recordingBank{}
	ctx, k := setupStoreKeeperWithOptions(t, WithBankKeeper(bank))
	auditor := testutil.AccAddress(t)
	record := auditorRecord(auditor)
	record.Status = vtypes.AuditorStatusResigned
	record.RenewalDeadline = time.Time{}
	record.BondStatus = vtypes.BondStatusUnbonding
	completion := ctx.BlockTime().Add(-time.Second)
	record.BondUnbondingCompletionTime = &completion

	require.NoError(t, k.SetAuditor(ctx, record))
	require.NoError(t, k.EndBlocker(ctx))

	got, found := k.GetAuditor(ctx, auditor)
	require.True(t, found)
	require.True(t, got.BondAmount.IsZero())
	require.Equal(t, vtypes.BondStatusUnspecified, got.BondStatus)
	require.Nil(t, got.BondUnbondingCompletionTime)
	require.Equal(t, []bankTransfer{
		{to: auditor, module: moduletypes.ModuleName, amt: sdk.NewCoins(record.BondAmount)},
	}, bank.moduleToAccount)
}

func TestEndBlockerSuspendsSnapshotCompliance(t *testing.T) {
	ctx, k := setupStoreKeeper(t)
	provider := testutil.AccAddress(t)
	record := providerSnapshotRecord(provider)
	record.ComplianceDeadline = ctx.BlockTime().Add(-time.Second)

	require.NoError(t, k.SetProviderSnapshot(ctx, record))
	require.NoError(t, k.EndBlocker(ctx))

	got, found := k.GetProviderSnapshot(ctx, provider)
	require.True(t, found)
	require.True(t, got.Suspended)
}

func TestEndBlockerExpiresAuditEscrowAndHonorsCap(t *testing.T) {
	bank := &recordingBank{}
	ctx, k := setupStoreKeeperWithOptions(t, WithBankKeeper(bank))
	params := k.GetParams(ctx)
	params.MaxEndblockerAuditEscrowExpiries = 1
	k.SetParams(ctx, params)

	provider1 := testutil.AccAddress(t)
	provider2 := testutil.AccAddress(t)
	escrow1 := openAuditEscrowRecord(ctx, provider1, 1, params)
	escrow2 := openAuditEscrowRecord(ctx, provider2, 2, params)
	escrow1.ExpiresAt = ctx.BlockTime().Add(-time.Second)
	escrow2.ExpiresAt = escrow1.ExpiresAt

	require.NoError(t, k.SetAuditEscrow(ctx, escrow2))
	require.NoError(t, k.SetAuditEscrow(ctx, escrow1))
	require.NoError(t, k.EndBlocker(ctx))

	got1, found := k.GetAuditEscrow(ctx, 1)
	require.True(t, found)
	require.Equal(t, vtypes.AuditEscrowStatusExpired, got1.Status)
	require.Equal(t, vtypes.FeeStatusReturnedToProvider, got1.FeeStatus)
	require.Equal(t, vtypes.ProviderDepositStatusReturnedToProvider, got1.ProviderDepositStatus)
	require.Equal(t, vtypes.AuditEscrowSettlementReasonExpiredUnconsumed, got1.SettlementReason)
	require.Equal(t, vtypes.FaultAttributionNoFault, got1.FaultAttribution)

	got2, found := k.GetAuditEscrow(ctx, 2)
	require.True(t, found)
	require.Equal(t, vtypes.AuditEscrowStatusOpen, got2.Status)
	require.Equal(t, []bankTransfer{
		{to: provider1, module: moduletypes.ModuleName, amt: sdk.NewCoins(params.MinFeeL1)},
		{to: provider1, module: moduletypes.ModuleName, amt: sdk.NewCoins(params.ProviderAuditDeposit)},
	}, bank.moduleToAccount)

	require.NoError(t, k.EndBlocker(ctx))
	got2, found = k.GetAuditEscrow(ctx, 2)
	require.True(t, found)
	require.Equal(t, vtypes.AuditEscrowStatusExpired, got2.Status)
}

func TestEndBlockerDoesNotExpireConsumedAuditEscrow(t *testing.T) {
	bank := &recordingBank{}
	ctx, k := setupStoreKeeperWithOptions(t, WithBankKeeper(bank))
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	escrow := auditEscrowRecord(provider, auditor)
	escrow.ExpiresAt = ctx.BlockTime().Add(-time.Second)
	consumedAt := ctx.BlockTime().Add(-2 * time.Second)
	escrow.ConsumedAt = &consumedAt

	require.NoError(t, k.SetAuditEscrow(ctx, escrow))
	require.NoError(t, k.EndBlocker(ctx))

	got, found := k.GetAuditEscrow(ctx, escrow.ID)
	require.True(t, found)
	require.Equal(t, vtypes.AuditEscrowStatusOpen, got.Status)
	require.Equal(t, auditor.String(), got.ConsumedByAuditor)
	require.Equal(t, consumedAt, *got.ConsumedAt)
	require.Empty(t, bank.moduleToAccount)
}

func TestEndBlockerExpiresVerificationGrace(t *testing.T) {
	ctx, k := setupStoreKeeper(t)
	provider := testutil.AccAddress(t)
	record := graceRecord(provider)
	record.ExpiresAt = ctx.BlockTime().Add(-time.Second)

	require.NoError(t, k.SetProviderVerificationGrace(ctx, record))
	require.NoError(t, k.EndBlocker(ctx))

	got, found := k.GetProviderVerificationGrace(ctx, provider)
	require.True(t, found)
	require.Equal(t, vtypes.VerificationGraceStatusExpired, got.Status)
}
