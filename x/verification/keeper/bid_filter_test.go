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

func TestBidFilterFallsThrough(t *testing.T) {
	ctx, k := setupBidFilterKeeper(t)
	provider := testutil.AccAddress(t)
	req := &vtypes.VerificationRequirement{MinTier: vtypes.TierVerified}

	require.NoError(t, k.BidFilter(ctx, provider, req))

	params := k.GetParams(ctx)
	params.VerificationModuleActive = true
	k.SetParams(ctx, params)

	require.NoError(t, k.BidFilter(ctx, provider, nil))
	require.NoError(t, k.BidFilter(ctx, provider, &vtypes.VerificationRequirement{
		MinTier:              vtypes.TierUnspecified,
		RequiredCapabilities: []vtypes.CapabilityFlag{vtypes.CapabilityBareMetal},
		RequiredAuditors:     []string{testutil.AccAddress(t).String()},
		MinAuditorCount:      1,
	}))
}

func TestBidFilterTierUsesBestValidAttestationAndActiveGrace(t *testing.T) {
	ctx, k := setupActiveBidFilterKeeper(t)
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	req := &vtypes.VerificationRequirement{MinTier: vtypes.TierEstablished}
	requireBidFilterSnapshot(t, ctx, k, provider)

	require.ErrorIs(t, k.BidFilter(ctx, provider, req), moduletypes.ErrInsufficientVerificationTier)

	record := attestationRecord(provider, auditor)
	record.Tier = vtypes.TierIdentified
	require.NoError(t, k.SetAttestation(ctx, record))
	require.ErrorIs(t, k.BidFilter(ctx, provider, req), moduletypes.ErrInsufficientVerificationTier)

	record.Tier = vtypes.TierEstablished
	require.NoError(t, k.SetAttestation(ctx, record))
	require.NoError(t, k.BidFilter(ctx, provider, req))

	record.Status = vtypes.AttestationStatusVoided
	require.NoError(t, k.SetAttestation(ctx, record))
	grace := vtypes.ProviderVerificationGraceRecord{
		ID:            1,
		Provider:      provider.String(),
		PreservedTier: vtypes.TierEstablished,
		StartedAt:     ctx.BlockTime(),
		ExpiresAt:     ctx.BlockTime().Add(DefaultParams().DiscrepancyGracePeriod),
		Status:        vtypes.VerificationGraceStatusActive,
	}
	require.NoError(t, k.SetProviderVerificationGrace(ctx, grace))
	require.NoError(t, k.BidFilter(ctx, provider, req))

	record.Status = vtypes.AttestationStatusValid
	record.Tier = vtypes.TierTrusted
	require.NoError(t, k.SetAttestation(ctx, record))
	grace.Status = vtypes.VerificationGraceStatusExpired
	require.NoError(t, k.SetProviderVerificationGrace(ctx, grace))
	require.NoError(t, k.SetProviderVerificationGrace(ctx, vtypes.ProviderVerificationGraceRecord{
		ID:            2,
		Provider:      provider.String(),
		PreservedTier: vtypes.TierIdentified,
		StartedAt:     ctx.BlockTime(),
		ExpiresAt:     ctx.BlockTime().Add(DefaultParams().DiscrepancyGracePeriod),
		Status:        vtypes.VerificationGraceStatusActive,
	}))
	require.NoError(t, k.BidFilter(ctx, provider, req))
}

func TestBidFilterGraceOnlySatisfiesTier(t *testing.T) {
	ctx, k := setupActiveBidFilterKeeper(t)
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	requireBidFilterSnapshot(t, ctx, k, provider)

	require.NoError(t, k.SetProviderVerificationGrace(ctx, vtypes.ProviderVerificationGraceRecord{
		ID:            1,
		Provider:      provider.String(),
		PreservedTier: vtypes.TierEstablished,
		StartedAt:     ctx.BlockTime(),
		ExpiresAt:     ctx.BlockTime().Add(DefaultParams().DiscrepancyGracePeriod),
		Status:        vtypes.VerificationGraceStatusActive,
	}))
	require.NoError(t, k.BidFilter(ctx, provider, &vtypes.VerificationRequirement{
		MinTier: vtypes.TierEstablished,
	}))

	require.ErrorIs(t, k.BidFilter(ctx, provider, &vtypes.VerificationRequirement{
		MinTier:              vtypes.TierEstablished,
		RequiredCapabilities: []vtypes.CapabilityFlag{vtypes.CapabilityBareMetal},
	}), moduletypes.ErrMissingCapability)
	require.ErrorIs(t, k.BidFilter(ctx, provider, &vtypes.VerificationRequirement{
		MinTier:          vtypes.TierEstablished,
		RequiredAuditors: []string{auditor.String()},
	}), moduletypes.ErrRequiredAuditorNotFound)
	require.ErrorIs(t, k.BidFilter(ctx, provider, &vtypes.VerificationRequirement{
		MinTier:         vtypes.TierEstablished,
		MinAuditorCount: 1,
	}), moduletypes.ErrInsufficientAuditorCount)
}

func TestBidFilterCapabilitiesAuditorsAndCount(t *testing.T) {
	ctx, k := setupActiveBidFilterKeeper(t)
	provider := testutil.AccAddress(t)
	auditorA := testutil.AccAddress(t)
	auditorB := testutil.AccAddress(t)
	requireBidFilterSnapshot(t, ctx, k, provider)

	recordA := attestationRecord(provider, auditorA)
	recordA.Tier = vtypes.TierTrusted
	recordA.Capabilities = []vtypes.CapabilityFlag{vtypes.CapabilityBareMetal}
	require.NoError(t, k.SetAttestation(ctx, recordA))

	baseReq := vtypes.VerificationRequirement{MinTier: vtypes.TierVerified}

	req := baseReq
	req.RequiredCapabilities = []vtypes.CapabilityFlag{vtypes.CapabilityConfidentialComputing}
	require.ErrorIs(t, k.BidFilter(ctx, provider, &req), moduletypes.ErrMissingCapability)

	req = baseReq
	req.RequiredCapabilities = []vtypes.CapabilityFlag{vtypes.CapabilityBareMetal}
	require.NoError(t, k.BidFilter(ctx, provider, &req))

	req = baseReq
	req.RequiredAuditors = []string{auditorB.String()}
	require.ErrorIs(t, k.BidFilter(ctx, provider, &req), moduletypes.ErrRequiredAuditorNotFound)

	req = baseReq
	req.RequiredAuditors = []string{auditorA.String(), auditorB.String()}
	req.AuditorMode = vtypes.AuditorSelectionModeAny
	require.NoError(t, k.BidFilter(ctx, provider, &req))

	req.AuditorMode = vtypes.AuditorSelectionModeAll
	require.ErrorIs(t, k.BidFilter(ctx, provider, &req), moduletypes.ErrRequiredAuditorNotFound)

	req = baseReq
	req.MinAuditorCount = 2
	require.ErrorIs(t, k.BidFilter(ctx, provider, &req), moduletypes.ErrInsufficientAuditorCount)

	recordB := attestationRecord(provider, auditorB)
	recordB.Tier = vtypes.TierVerified
	recordB.Status = vtypes.AttestationStatusVoided
	require.NoError(t, k.SetAttestation(ctx, recordB))

	req = baseReq
	req.RequiredAuditors = []string{auditorB.String()}
	require.ErrorIs(t, k.BidFilter(ctx, provider, &req), moduletypes.ErrRequiredAuditorNotFound)

	req = baseReq
	req.MinAuditorCount = 2
	require.ErrorIs(t, k.BidFilter(ctx, provider, &req), moduletypes.ErrInsufficientAuditorCount)

	recordB.Status = vtypes.AttestationStatusValid
	require.NoError(t, k.SetAttestation(ctx, recordB))

	req = baseReq
	req.RequiredAuditors = []string{auditorA.String(), auditorB.String()}
	req.AuditorMode = vtypes.AuditorSelectionModeAll
	req.MinAuditorCount = 2
	require.NoError(t, k.BidFilter(ctx, provider, &req))

	recordB.Tier = vtypes.TierIdentified
	require.NoError(t, k.SetAttestation(ctx, recordB))
	require.ErrorIs(t, k.BidFilter(ctx, provider, &req), moduletypes.ErrInsufficientAuditorCount)
}

func TestBidFilterRequiresSnapshotForL2Plus(t *testing.T) {
	ctx, k := setupActiveBidFilterKeeper(t)
	provider := testutil.AccAddress(t)

	require.ErrorIs(t, k.BidFilter(ctx, provider, &vtypes.VerificationRequirement{
		MinTier: vtypes.TierVerified,
	}), moduletypes.ErrSnapshotNonCompliant)

	snapshot := providerSnapshotRecord(provider)
	snapshot.ComplianceDeadline = ctx.BlockTime().Add(-time.Second)
	require.NoError(t, k.SetProviderSnapshot(ctx, snapshot))
	require.ErrorIs(t, k.BidFilter(ctx, provider, &vtypes.VerificationRequirement{
		MinTier: vtypes.TierVerified,
	}), moduletypes.ErrProviderSnapshotSuspended)

	snapshot.ComplianceDeadline = ctx.BlockTime().Add(time.Hour)
	require.NoError(t, k.SetProviderSnapshot(ctx, snapshot))
	require.ErrorIs(t, k.BidFilter(ctx, provider, &vtypes.VerificationRequirement{
		MinTier: vtypes.TierVerified,
	}), moduletypes.ErrInsufficientVerificationTier)
}

func requireBidFilterSnapshot(t testing.TB, ctx sdk.Context, k *keeper, provider sdk.AccAddress) {
	t.Helper()
	snapshot := providerSnapshotRecord(provider)
	snapshot.ComplianceDeadline = ctx.BlockTime().Add(time.Hour)
	require.NoError(t, k.SetProviderSnapshot(ctx, snapshot))
}

func setupBidFilterKeeper(t testing.TB) (sdk.Context, *keeper) {
	t.Helper()
	ctx, k := setupStoreKeeper(t)
	return ctx, k.(*keeper)
}

func setupActiveBidFilterKeeper(t testing.TB) (sdk.Context, *keeper) {
	t.Helper()
	ctx, k := setupBidFilterKeeper(t)
	params := k.GetParams(ctx)
	params.VerificationModuleActive = true
	k.SetParams(ctx, params)
	return ctx, k
}
