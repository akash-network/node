package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	vtypes "pkg.akt.dev/go/node/verification/v1"

	moduletypes "pkg.akt.dev/node/v3/x/verification/types"
)

// BidFilter enforces deployment verification requirements for market bids.
func (k *keeper) BidFilter(ctx sdk.Context, provider sdk.AccAddress, req *vtypes.VerificationRequirement) error {
	params := k.GetParams(ctx)
	if !params.GetVerificationModuleActive() || !bidFilterRequiresVerification(req) {
		return nil
	}
	if err := k.bidFilterSnapshotCompliant(ctx, provider, req.GetMinTier()); err != nil {
		return err
	}

	state := k.bidFilterState(ctx, provider, req.GetMinTier())
	if !vtypes.TierAtLeast(state.tier, req.GetMinTier()) {
		return moduletypes.ErrInsufficientVerificationTier
	}

	if !bidFilterCapabilitiesSatisfied(req.GetRequiredCapabilities(), state.capabilities) {
		return moduletypes.ErrMissingCapability
	}

	if uint32(len(state.auditors)) < req.GetMinAuditorCount() {
		return moduletypes.ErrInsufficientAuditorCount
	}

	if !bidFilterAuditorsSatisfied(req.GetRequiredAuditors(), req.GetAuditorMode(), state.auditors) {
		return moduletypes.ErrRequiredAuditorNotFound
	}

	return nil
}

func (k *keeper) bidFilterSnapshotCompliant(ctx sdk.Context, provider sdk.AccAddress, tier vtypes.VerificationTier) error {
	if !vtypes.TierAtLeast(tier, vtypes.TierVerified) {
		return nil
	}

	snapshot, found := k.GetProviderSnapshot(ctx, provider)
	if !found {
		return moduletypes.ErrSnapshotNonCompliant
	}
	if snapshot.GetSuspended() || !snapshot.GetComplianceDeadline().After(ctx.BlockTime()) {
		return moduletypes.ErrProviderSnapshotSuspended
	}
	return nil
}

func bidFilterRequiresVerification(req *vtypes.VerificationRequirement) bool {
	return req != nil && req.GetMinTier() != vtypes.TierUnspecified
}

type bidFilterState struct {
	tier         vtypes.VerificationTier
	capabilities map[vtypes.CapabilityFlag]struct{}
	auditors     map[string]struct{}
}

func (k *keeper) bidFilterState(ctx sdk.Context, provider sdk.AccAddress, minTier vtypes.VerificationTier) bidFilterState {
	state := bidFilterState{
		tier:         vtypes.TierUnspecified,
		capabilities: make(map[vtypes.CapabilityFlag]struct{}),
		auditors:     make(map[string]struct{}),
	}

	k.WithProviderAttestations(ctx, provider, vtypes.AttestationStatusValid, func(record vtypes.AttestationRecord) bool {
		if vtypes.TierBetter(record.GetTier(), state.tier) {
			state.tier = record.GetTier()
		}
		for _, capability := range record.GetCapabilities() {
			state.capabilities[capability] = struct{}{}
		}
		if auditor := record.GetAuditor(); auditor != "" && vtypes.TierAtLeast(record.GetTier(), minTier) {
			state.auditors[auditor] = struct{}{}
		}
		return false
	})

	k.WithProviderVerificationGraces(ctx, provider, func(record vtypes.ProviderVerificationGraceRecord) bool {
		if record.GetStatus() == vtypes.VerificationGraceStatusActive && vtypes.TierBetter(record.GetPreservedTier(), state.tier) {
			state.tier = record.GetPreservedTier()
			return true
		}
		return false
	})

	return state
}

func bidFilterCapabilitiesSatisfied(required []vtypes.CapabilityFlag, have map[vtypes.CapabilityFlag]struct{}) bool {
	for _, capability := range required {
		if capability == vtypes.CapabilityUnspecified {
			continue
		}
		if _, exists := have[capability]; !exists {
			return false
		}
	}
	return true
}

func bidFilterAuditorsSatisfied(required []string, mode vtypes.AuditorSelectionMode, have map[string]struct{}) bool {
	if len(required) == 0 {
		return true
	}

	if mode == vtypes.AuditorSelectionModeAll {
		for _, auditor := range required {
			if _, exists := have[auditor]; !exists {
				return false
			}
		}
		return true
	}

	for _, auditor := range required {
		if _, exists := have[auditor]; exists {
			return true
		}
	}
	return false
}
