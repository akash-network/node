package keeper

import (
	"context"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	mv1 "pkg.akt.dev/go/node/market/v1"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	vtypes "pkg.akt.dev/go/node/verification/v1"
	"pkg.akt.dev/go/testutil"

	moduletypes "pkg.akt.dev/node/v3/x/verification/types"
)

func TestHappyPathMessages(t *testing.T) {
	bank := &recordingBank{}
	ctx, k := setupStoreKeeperWithOptions(t, WithAuthority("gov"), WithBankKeeper(bank))
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	params := k.GetParams(ctx)

	err := k.RegisterAuditor(ctx, "gov", auditor, vtypes.TierTrusted, []byte("auditor-meta"))
	require.NoError(t, err)

	err = k.PostAuditorBond(ctx, auditor, params.BondL4)
	require.NoError(t, err)

	err = k.PostProviderBond(ctx, provider, sdk.NewInt64Coin(bondDenom, 200000000))
	require.NoError(t, err)

	resources := vtypes.ResourceSummary{
		TotalGPUs:      2,
		TotalVCPUs:     4,
		TotalMemoryMB:  2048,
		TotalStorageMB: 1048576,
	}
	err = k.PostSnapshotHash(ctx, provider, testHash(), resources, ctx.BlockTime())
	require.NoError(t, err)

	escrowID, err := k.OpenAuditEscrow(
		ctx,
		provider,
		vtypes.TierVerified,
		[]vtypes.CapabilityFlag{vtypes.CapabilityTEEHardwareAttestation},
		params.MinFeeL2,
		params.ProviderAuditDeposit,
		ctx.BlockTime().Add(params.TtlL2),
		[]byte("escrow-meta"),
	)
	require.NoError(t, err)
	require.Equal(t, uint64(1), escrowID)

	err = k.SubmitAttestation(
		ctx,
		provider,
		auditor,
		vtypes.TierVerified,
		[]vtypes.CapabilityFlag{vtypes.CapabilityTEEHardwareAttestation},
		testHash(),
		params.MinFeeL2,
		params.AttestationDeposit,
		escrowID,
	)
	require.NoError(t, err)

	attestation, found := k.GetAttestation(ctx, provider, auditor)
	require.True(t, found)
	require.Equal(t, vtypes.AttestationStatusValid, attestation.Status)
	require.Equal(t, vtypes.TierVerified, attestation.Tier)

	escrow, found := k.GetAuditEscrow(ctx, escrowID)
	require.True(t, found)
	require.Equal(t, vtypes.AuditEscrowStatusConsumed, escrow.Status)
	require.Equal(t, auditor.String(), escrow.ConsumedByAuditor)
	require.NotNil(t, escrow.ConsumedAt)

	events := ctx.EventManager().Events().ToABCIEvents()
	testutil.EnsureEvent(t, events, &vtypes.EventAuditorBondPosted{
		Auditor: auditor.String(),
		Amount:  params.BondL4,
	})
	testutil.EnsureEvent(t, events, &vtypes.EventProviderBondPosted{
		Provider:    provider.String(),
		Amount:      sdk.NewInt64Coin(bondDenom, 200000000),
		TotalBonded: sdk.NewInt64Coin(bondDenom, 200000000),
	})
	testutil.EnsureEvent(t, events, &vtypes.EventSnapshotHashPosted{
		Provider:           provider.String(),
		SnapshotHash:       testHash(),
		ComplianceDeadline: ctx.BlockTime().Add(params.SnapshotHashInterval),
	})
	testutil.EnsureEvent(t, events, &vtypes.EventAuditEscrowOpened{
		AuditEscrowID:   escrowID,
		Provider:        provider.String(),
		Fee:             params.MinFeeL2,
		ProviderDeposit: params.ProviderAuditDeposit,
	})
	testutil.EnsureEvent(t, events, &vtypes.EventAttestationSubmitted{
		Provider:      provider.String(),
		Auditor:       auditor.String(),
		Tier:          vtypes.TierVerified,
		Capabilities:  []vtypes.CapabilityFlag{vtypes.CapabilityTEEHardwareAttestation},
		ExpiresAt:     ctx.BlockTime().Add(params.TtlL2),
		AuditEscrowID: escrowID,
	})

	require.Len(t, bank.accountToModule, 5)
}

func TestRegisterAuditorCreatesPendingBondRecord(t *testing.T) {
	ctx, k := setupStoreKeeperWithOptions(t, WithAuthority("gov"))
	auditor := testutil.AccAddress(t)
	params := k.GetParams(ctx)

	err := k.RegisterAuditor(ctx, "gov", auditor, vtypes.TierVerified, []byte("auditor-meta"))
	require.NoError(t, err)

	record, found := k.GetAuditor(ctx, auditor)
	require.True(t, found)
	require.Equal(t, vtypes.AuditorStatusPendingBond, record.Status)
	require.Equal(t, vtypes.BondStatusNotBonded, record.BondStatus)
	require.True(t, record.BondAmount.IsZero())
	require.Equal(t, sdk.NewCoin(params.BondL1.Denom, sdkmath.ZeroInt()), record.BondAmount)
}

func TestPostAuditorBondActivatesAfterRequiredBond(t *testing.T) {
	ctx, k := setupStoreKeeperWithOptions(t, WithAuthority("gov"))
	auditor := testutil.AccAddress(t)
	params := k.GetParams(ctx)

	err := k.RegisterAuditor(ctx, "gov", auditor, vtypes.TierVerified, nil)
	require.NoError(t, err)

	err = k.PostAuditorBond(ctx, auditor, params.BondL1)
	require.NoError(t, err)

	record, found := k.GetAuditor(ctx, auditor)
	require.True(t, found)
	require.Equal(t, vtypes.AuditorStatusPendingBond, record.Status)
	require.Equal(t, vtypes.BondStatusNotBonded, record.BondStatus)
	require.Equal(t, params.BondL1, record.BondAmount)

	err = k.PostAuditorBond(ctx, auditor, params.BondL2)
	require.NoError(t, err)

	record, found = k.GetAuditor(ctx, auditor)
	require.True(t, found)
	require.Equal(t, vtypes.AuditorStatusActive, record.Status)
	require.Equal(t, vtypes.BondStatusBonded, record.BondStatus)
	require.True(t, record.BondAmount.Amount.GTE(params.BondL2.Amount))
}

func TestSubmitAttestationRejectsAuditorWithoutBondedStatus(t *testing.T) {
	ctx, k := setupStoreKeeper(t)
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	params := k.GetParams(ctx)

	require.NoError(t, k.SetAuditor(ctx, vtypes.AuditorRecord{
		Address:            auditor.String(),
		Status:             vtypes.AuditorStatusActive,
		MaxAttestationTier: vtypes.TierIdentified,
		BondAmount:         params.BondL1,
		BondStatus:         vtypes.BondStatusNotBonded,
		RegisteredAt:       ctx.BlockTime(),
		RenewalDeadline:    ctx.BlockTime().Add(params.RenewalPeriodL1),
	}))
	require.NoError(t, k.SetAuditEscrow(ctx, openAuditEscrowRecord(ctx, provider, 1, params)))

	err := k.SubmitAttestation(
		ctx,
		provider,
		auditor,
		vtypes.TierIdentified,
		nil,
		testHash(),
		params.MinFeeL1,
		params.AttestationDeposit,
		1,
	)
	require.ErrorIs(t, err, moduletypes.ErrInsufficientAuditorBond)
}

func TestSubmitAttestationConsumesCoveredEscrowFee(t *testing.T) {
	bank := &recordingBank{}
	ctx, k := setupStoreKeeperWithOptions(t, WithBankKeeper(bank))
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	params := k.GetParams(ctx)

	require.NoError(t, k.SetAuditor(ctx, auditorRecord(auditor)))
	require.NoError(t, k.SetProviderBond(ctx, vtypes.ProviderBondRecord{
		Provider:     provider.String(),
		BondedAmount: params.BondL4,
	}))
	require.NoError(t, k.SetProviderSnapshot(ctx, providerSnapshotRecord(provider)))

	escrow := openAuditEscrowRecord(ctx, provider, 1, params)
	escrow.RequestedTier = vtypes.TierVerified
	escrow.Fee = sdk.NewCoin(params.MinFeeL2.Denom, params.MinFeeL2.Amount.AddRaw(1000))
	require.NoError(t, k.SetAuditEscrow(ctx, escrow))

	err := k.SubmitAttestation(
		ctx,
		provider,
		auditor,
		vtypes.TierVerified,
		nil,
		testHash(),
		params.MinFeeL2,
		params.AttestationDeposit,
		1,
	)
	require.NoError(t, err)

	gotEscrow, found := k.GetAuditEscrow(ctx, 1)
	require.True(t, found)
	require.Equal(t, params.MinFeeL2, gotEscrow.Fee)

	gotAttestation, found := k.GetAttestation(ctx, provider, auditor)
	require.True(t, found)
	require.Equal(t, params.MinFeeL2, gotAttestation.Fee)
	require.Equal(t, []bankTransfer{
		{to: provider, module: moduletypes.ModuleName, amt: sdk.NewCoins(sdk.NewInt64Coin(bondDenom, 1000))},
	}, bank.moduleToAccount)
}

func TestSubmitAttestationRequiresSnapshotForL2(t *testing.T) {
	ctx, k := setupStoreKeeper(t)
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	params := k.GetParams(ctx)

	require.NoError(t, k.SetAuditor(ctx, vtypes.AuditorRecord{
		Address:            auditor.String(),
		Status:             vtypes.AuditorStatusActive,
		MaxAttestationTier: vtypes.TierTrusted,
		BondAmount:         params.BondL4,
		BondStatus:         vtypes.BondStatusBonded,
		RegisteredAt:       ctx.BlockTime(),
		RenewalDeadline:    ctx.BlockTime().Add(params.RenewalPeriodL4),
	}))
	require.NoError(t, k.SetAuditEscrow(ctx, vtypes.AuditEscrowRecord{
		ID:                    1,
		Provider:              provider.String(),
		RequestedTier:         vtypes.TierVerified,
		Fee:                   params.MinFeeL2,
		FeeStatus:             vtypes.FeeStatusEscrowed,
		ProviderDeposit:       params.ProviderAuditDeposit,
		ProviderDepositStatus: vtypes.ProviderDepositStatusEscrowed,
		Status:                vtypes.AuditEscrowStatusOpen,
		OpenedAt:              ctx.BlockTime(),
		ExpiresAt:             ctx.BlockTime().Add(params.TtlL2),
	}))

	err := k.SubmitAttestation(
		ctx,
		provider,
		auditor,
		vtypes.TierVerified,
		nil,
		testHash(),
		params.MinFeeL2,
		params.AttestationDeposit,
		1,
	)
	require.ErrorIs(t, err, moduletypes.ErrSnapshotNonCompliant)
}

func TestSubmitAttestationRejectsProviderTooYoungForL2(t *testing.T) {
	ctx, k, provider, auditor, params := setupL2AttestationPrerequisites(t, func(ctx sdk.Context, params vtypes.Params) time.Time {
		return ctx.BlockTime().Add(-params.MinAgeL2).Add(time.Second)
	})

	err := k.SubmitAttestation(
		ctx,
		provider,
		auditor,
		vtypes.TierVerified,
		nil,
		testHash(),
		params.MinFeeL2,
		params.AttestationDeposit,
		1,
	)
	require.ErrorIs(t, err, moduletypes.ErrInsufficientProviderAge)
}

func TestSubmitAttestationAcceptsProviderOldEnoughForL2(t *testing.T) {
	ctx, k, provider, auditor, params := setupL2AttestationPrerequisites(t, func(ctx sdk.Context, params vtypes.Params) time.Time {
		return ctx.BlockTime().Add(-params.MinAgeL2)
	})

	err := k.SubmitAttestation(
		ctx,
		provider,
		auditor,
		vtypes.TierVerified,
		nil,
		testHash(),
		params.MinFeeL2,
		params.AttestationDeposit,
		1,
	)
	require.NoError(t, err)
}

func TestSubmitAttestationRejectsInsufficientLeaseHistoryForL3(t *testing.T) {
	tests := []struct {
		name      string
		completed uint64
		failures  map[mv1.LeaseClosedReason]uint64
		found     bool
	}{
		{
			name:      "below completion rate",
			completed: 97,
			failures: map[mv1.LeaseClosedReason]uint64{
				mv1.LeaseClosedReasonUnstable: 3,
			},
			found: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, k, provider, auditor, params := setupL3AttestationPrerequisites(t, stubMarketStatsKeeper{
				completed: tc.completed,
				failures:  tc.failures,
				found:     tc.found,
			})

			err := k.SubmitAttestation(
				ctx,
				provider,
				auditor,
				vtypes.TierEstablished,
				nil,
				testHash(),
				params.MinFeeL3,
				params.AttestationDeposit,
				1,
			)
			require.ErrorIs(t, err, moduletypes.ErrInsufficientLeaseCompletionRate)
		})
	}
}

func TestSubmitAttestationSkipsLeaseHistoryForLowVolume(t *testing.T) {
	tests := []struct {
		name      string
		completed uint64
		failures  map[mv1.LeaseClosedReason]uint64
		found     bool
	}{
		{
			name:      "no stats",
			completed: 0,
			failures:  nil,
			found:     false,
		},
		{
			name:      "below minimum leases",
			completed: 9,
			failures:  nil,
			found:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, k, provider, auditor, params := setupL3AttestationPrerequisites(t, stubMarketStatsKeeper{
				completed: tc.completed,
				failures:  tc.failures,
				found:     tc.found,
			})

			err := k.SubmitAttestation(
				ctx,
				provider,
				auditor,
				vtypes.TierEstablished,
				nil,
				testHash(),
				params.MinFeeL3,
				params.AttestationDeposit,
				1,
			)
			require.NoError(t, err)
		})
	}
}

func TestSubmitAttestationAcceptsSufficientLeaseHistoryForL3(t *testing.T) {
	ctx, k, provider, auditor, params := setupL3AttestationPrerequisites(t, stubMarketStatsKeeper{
		completed: 98,
		failures: map[mv1.LeaseClosedReason]uint64{
			mv1.LeaseClosedReasonUnstable: 2,
		},
		found: true,
	})

	err := k.SubmitAttestation(
		ctx,
		provider,
		auditor,
		vtypes.TierEstablished,
		nil,
		testHash(),
		params.MinFeeL3,
		params.AttestationDeposit,
		1,
	)
	require.NoError(t, err)
}

func TestSubmitAttestationRejectsRecentProviderSlashForL3(t *testing.T) {
	ctx, k, provider, auditor, params := setupL3AttestationPrerequisites(t, stubMarketStatsKeeper{
		completed: 98,
		failures: map[mv1.LeaseClosedReason]uint64{
			mv1.LeaseClosedReasonUnstable: 2,
		},
		found: true,
	})
	bond, found := k.GetProviderBond(ctx, provider)
	require.True(t, found)
	slashedAt := ctx.BlockTime().Add(-params.CleanHistoryWindowL3).Add(time.Second)
	bond.LastSlashTime = &slashedAt
	require.NoError(t, k.SetProviderBond(ctx, bond))

	err := k.SubmitAttestation(
		ctx,
		provider,
		auditor,
		vtypes.TierEstablished,
		nil,
		testHash(),
		params.MinFeeL3,
		params.AttestationDeposit,
		1,
	)
	require.ErrorIs(t, err, moduletypes.ErrSlashingHistoryViolation)
}

func TestSubmitAttestationRejectsInsufficientL3HistoryForL4(t *testing.T) {
	ctx, k, provider, auditor, params := setupL4AttestationPrerequisites(t, false)

	err := k.SubmitAttestation(
		ctx,
		provider,
		auditor,
		vtypes.TierTrusted,
		nil,
		testHash(),
		params.MinFeeL4,
		params.AttestationDeposit,
		1,
	)
	require.ErrorIs(t, err, moduletypes.ErrInsufficientL3History)
}

func TestSubmitAttestationAcceptsContinuousL3HistoryForL4(t *testing.T) {
	ctx, k, provider, auditor, params := setupL4AttestationPrerequisites(t, true)

	err := k.SubmitAttestation(
		ctx,
		provider,
		auditor,
		vtypes.TierTrusted,
		nil,
		testHash(),
		params.MinFeeL4,
		params.AttestationDeposit,
		1,
	)
	require.NoError(t, err)
}

func TestSubmitAttestationRejectsSelfAttestation(t *testing.T) {
	ctx, k := setupStoreKeeper(t)
	provider := testutil.AccAddress(t)
	params := k.GetParams(ctx)

	err := k.SubmitAttestation(
		ctx,
		provider,
		provider,
		vtypes.TierIdentified,
		nil,
		testHash(),
		params.MinFeeL1,
		params.AttestationDeposit,
		1,
	)
	require.ErrorIs(t, err, moduletypes.ErrSelfAttestation)
}

func TestSubmitAttestationSameAuditorReplacementDisposition(t *testing.T) {
	tests := []struct {
		name                  string
		existingFeeStatus     vtypes.FeeStatus
		existingDepositStatus vtypes.DepositStatus
		wantErr               error
		wantStatus            vtypes.AttestationStatus
		wantFeeStatus         vtypes.FeeStatus
		wantDepositStatus     vtypes.DepositStatus
	}{
		{
			name:                  "settles existing valid attestation and stores replacement",
			existingFeeStatus:     vtypes.FeeStatusEscrowed,
			existingDepositStatus: vtypes.DepositStatusEscrowed,
			wantStatus:            vtypes.AttestationStatusValid,
			wantFeeStatus:         vtypes.FeeStatusEscrowed,
			wantDepositStatus:     vtypes.DepositStatusEscrowed,
		},
		{
			name:                  "rejects replacement when existing funds are not escrowed",
			existingFeeStatus:     vtypes.FeeStatusReleasedToAuditor,
			existingDepositStatus: vtypes.DepositStatusEscrowed,
			wantErr:               moduletypes.ErrInvalidReason,
			wantStatus:            vtypes.AttestationStatusValid,
			wantFeeStatus:         vtypes.FeeStatusReleasedToAuditor,
			wantDepositStatus:     vtypes.DepositStatusEscrowed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bank := &recordingBank{}
			ctx, k := setupStoreKeeperWithOptions(t, WithBankKeeper(bank))
			provider := testutil.AccAddress(t)
			auditor := testutil.AccAddress(t)
			params := k.GetParams(ctx)

			require.NoError(t, k.SetAuditor(ctx, auditorRecord(auditor)))
			existing := attestationRecord(provider, auditor)
			existing.Tier = vtypes.TierIdentified
			existing.Fee = params.MinFeeL1
			existing.FeeStatus = tc.existingFeeStatus
			existing.Deposit = params.AttestationDeposit
			existing.DepositStatus = tc.existingDepositStatus
			require.NoError(t, k.SetAttestation(ctx, existing))
			require.NoError(t, k.SetAuditEscrow(ctx, openAuditEscrowRecord(ctx, provider, 1, params)))

			err := k.SubmitAttestation(
				ctx,
				provider,
				auditor,
				vtypes.TierIdentified,
				nil,
				testHash(),
				params.MinFeeL1,
				params.AttestationDeposit,
				1,
			)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Empty(t, bank.moduleToAccount)
				require.Empty(t, bank.accountToModule)
			} else {
				require.NoError(t, err)
				events := ctx.EventManager().Events().ToABCIEvents()
				testutil.EnsureEvent(t, events, &vtypes.EventAttestationReplaced{
					Provider:         provider.String(),
					Auditor:          auditor.String(),
					OldTier:          existing.Tier,
					NewTier:          vtypes.TierIdentified,
					OldAuditEscrowID: existing.AuditEscrowID,
					NewAuditEscrowID: 1,
				})
				require.Equal(t, []bankTransfer{
					{to: auditor, module: moduletypes.ModuleName, amt: sdk.NewCoins(params.MinFeeL1)},
					{to: auditor, module: moduletypes.ModuleName, amt: sdk.NewCoins(params.AttestationDeposit)},
				}, bank.moduleToAccount)
				require.Equal(t, []bankTransfer{
					{from: auditor, module: moduletypes.ModuleName, amt: sdk.NewCoins(params.AttestationDeposit)},
				}, bank.accountToModule)
			}

			got, found := k.GetAttestation(ctx, provider, auditor)
			require.True(t, found)
			require.Equal(t, tc.wantStatus, got.Status)
			require.Equal(t, tc.wantFeeStatus, got.FeeStatus)
			require.Equal(t, tc.wantDepositStatus, got.DepositStatus)
		})
	}
}

func TestRevokeAttestationDisposition(t *testing.T) {
	tests := []struct {
		name              string
		reason            vtypes.AttestationRevocationReason
		wantFault         vtypes.FaultAttribution
		wantFeeStatus     vtypes.FeeStatus
		wantDepositStatus vtypes.DepositStatus
		wantAccountTxs    []func(sdk.AccAddress, sdk.AccAddress, vtypes.AttestationRecord) bankTransfer
		wantModuleTxs     []func(vtypes.AttestationRecord) bankTransfer
	}{
		{
			name:              "provider fault releases fee and deposit to auditor",
			reason:            vtypes.AttestationRevocationReasonProviderNoLongerQualifies,
			wantFault:         vtypes.FaultAttributionProviderFault,
			wantFeeStatus:     vtypes.FeeStatusReleasedToAuditor,
			wantDepositStatus: vtypes.DepositStatusReturnedToAuditor,
			wantAccountTxs: []func(sdk.AccAddress, sdk.AccAddress, vtypes.AttestationRecord) bankTransfer{
				func(_ sdk.AccAddress, auditor sdk.AccAddress, attestation vtypes.AttestationRecord) bankTransfer {
					return bankTransfer{to: auditor, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestation.Fee)}
				},
				func(_ sdk.AccAddress, auditor sdk.AccAddress, attestation vtypes.AttestationRecord) bankTransfer {
					return bankTransfer{to: auditor, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestation.Deposit)}
				},
			},
		},
		{
			name:              "auditor evidence error refunds provider and slashes deposit",
			reason:            vtypes.AttestationRevocationReasonAuditorEvidenceError,
			wantFault:         vtypes.FaultAttributionAuditorFault,
			wantFeeStatus:     vtypes.FeeStatusReturnedToProvider,
			wantDepositStatus: vtypes.DepositStatusSlashed,
			wantAccountTxs: []func(sdk.AccAddress, sdk.AccAddress, vtypes.AttestationRecord) bankTransfer{
				func(provider sdk.AccAddress, _ sdk.AccAddress, attestation vtypes.AttestationRecord) bankTransfer {
					return bankTransfer{to: provider, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestation.Fee)}
				},
			},
			wantModuleTxs: []func(vtypes.AttestationRecord) bankTransfer{
				func(attestation vtypes.AttestationRecord) bankTransfer {
					return bankTransfer{fromModule: moduletypes.ModuleName, toModule: distrtypes.ModuleName, amt: sdk.NewCoins(attestation.Deposit)}
				},
			},
		},
		{
			name:              "auditor operational exit refunds provider and returns deposit",
			reason:            vtypes.AttestationRevocationReasonAuditorOperationalExit,
			wantFault:         vtypes.FaultAttributionNoFault,
			wantFeeStatus:     vtypes.FeeStatusReturnedToProvider,
			wantDepositStatus: vtypes.DepositStatusReturnedToAuditor,
			wantAccountTxs: []func(sdk.AccAddress, sdk.AccAddress, vtypes.AttestationRecord) bankTransfer{
				func(provider sdk.AccAddress, _ sdk.AccAddress, attestation vtypes.AttestationRecord) bankTransfer {
					return bankTransfer{to: provider, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestation.Fee)}
				},
				func(_ sdk.AccAddress, auditor sdk.AccAddress, attestation vtypes.AttestationRecord) bankTransfer {
					return bankTransfer{to: auditor, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestation.Deposit)}
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bank := &recordingBank{}
			ctx, k := setupStoreKeeperWithOptions(t, WithBankKeeper(bank))
			provider := testutil.AccAddress(t)
			auditor := testutil.AccAddress(t)
			attestation := attestationRecord(provider, auditor)
			require.NoError(t, k.SetAttestation(ctx, attestation))

			err := k.RevokeAttestation(ctx, provider, auditor, tc.reason, testHash())
			require.NoError(t, err)

			got, found := k.GetAttestation(ctx, provider, auditor)
			require.True(t, found)
			require.Equal(t, vtypes.AttestationStatusRevoked, got.Status)
			require.Equal(t, tc.wantFault, got.FaultAttribution)
			require.Equal(t, tc.wantFeeStatus, got.FeeStatus)
			require.Equal(t, tc.wantDepositStatus, got.DepositStatus)

			wantAccountTxs := make([]bankTransfer, 0, len(tc.wantAccountTxs))
			for _, build := range tc.wantAccountTxs {
				wantAccountTxs = append(wantAccountTxs, build(provider, auditor, attestation))
			}
			wantModuleTxs := make([]bankTransfer, 0, len(tc.wantModuleTxs))
			for _, build := range tc.wantModuleTxs {
				wantModuleTxs = append(wantModuleTxs, build(attestation))
			}
			require.Equal(t, wantAccountTxs, bank.moduleToAccount)
			if len(wantModuleTxs) == 0 {
				require.Empty(t, bank.moduleToModule)
			} else {
				require.Equal(t, wantModuleTxs, bank.moduleToModule)
			}
		})
	}
}

func TestRevokeAttestationRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(sdk.Context, Keeper, sdk.AccAddress, sdk.AccAddress)
		provider  func(sdk.AccAddress) sdk.AccAddress
		auditor   func(sdk.AccAddress) sdk.AccAddress
		reason    vtypes.AttestationRevocationReason
		evidence  []byte
		wantErr   error
		wantState vtypes.AttestationStatus
	}{
		{
			name: "wrong auditor has no authorization over record",
			setup: func(ctx sdk.Context, k Keeper, provider sdk.AccAddress, auditor sdk.AccAddress) {
				require.NoError(t, k.SetAttestation(ctx, attestationRecord(provider, auditor)))
			},
			provider: func(provider sdk.AccAddress) sdk.AccAddress { return provider },
			auditor:  func(sdk.AccAddress) sdk.AccAddress { return testutil.AccAddress(t) },
			reason:   vtypes.AttestationRevocationReasonProviderNoLongerQualifies,
			evidence: testHash(),
			wantErr:  moduletypes.ErrAttestationNotFound,
		},
		{
			name: "unspecified reason",
			setup: func(ctx sdk.Context, k Keeper, provider sdk.AccAddress, auditor sdk.AccAddress) {
				require.NoError(t, k.SetAttestation(ctx, attestationRecord(provider, auditor)))
			},
			provider:  func(provider sdk.AccAddress) sdk.AccAddress { return provider },
			auditor:   func(auditor sdk.AccAddress) sdk.AccAddress { return auditor },
			reason:    vtypes.AttestationRevocationReasonUnspecified,
			evidence:  testHash(),
			wantErr:   moduletypes.ErrInvalidReason,
			wantState: vtypes.AttestationStatusValid,
		},
		{
			name: "malformed evidence hash",
			setup: func(ctx sdk.Context, k Keeper, provider sdk.AccAddress, auditor sdk.AccAddress) {
				require.NoError(t, k.SetAttestation(ctx, attestationRecord(provider, auditor)))
			},
			provider:  func(provider sdk.AccAddress) sdk.AccAddress { return provider },
			auditor:   func(auditor sdk.AccAddress) sdk.AccAddress { return auditor },
			reason:    vtypes.AttestationRevocationReasonProviderNoLongerQualifies,
			evidence:  []byte("short"),
			wantErr:   moduletypes.ErrInvalidReason,
			wantState: vtypes.AttestationStatusValid,
		},
		{
			name: "already removed attestation",
			setup: func(ctx sdk.Context, k Keeper, provider sdk.AccAddress, auditor sdk.AccAddress) {
				attestation := attestationRecord(provider, auditor)
				attestation.Status = vtypes.AttestationStatusRemoved
				require.NoError(t, k.SetAttestation(ctx, attestation))
			},
			provider:  func(provider sdk.AccAddress) sdk.AccAddress { return provider },
			auditor:   func(auditor sdk.AccAddress) sdk.AccAddress { return auditor },
			reason:    vtypes.AttestationRevocationReasonProviderNoLongerQualifies,
			evidence:  testHash(),
			wantErr:   moduletypes.ErrInvalidReason,
			wantState: vtypes.AttestationStatusRemoved,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bank := &recordingBank{}
			ctx, k := setupStoreKeeperWithOptions(t, WithBankKeeper(bank))
			provider := testutil.AccAddress(t)
			auditor := testutil.AccAddress(t)
			tc.setup(ctx, k, provider, auditor)

			err := k.RevokeAttestation(ctx, tc.provider(provider), tc.auditor(auditor), tc.reason, tc.evidence)
			require.ErrorIs(t, err, tc.wantErr)
			require.Empty(t, bank.moduleToAccount)
			require.Empty(t, bank.moduleToModule)

			if tc.wantState != vtypes.AttestationStatusUnspecified {
				got, found := k.GetAttestation(ctx, provider, auditor)
				require.True(t, found)
				require.Equal(t, tc.wantState, got.Status)
				require.Equal(t, vtypes.FeeStatusEscrowed, got.FeeStatus)
				require.Equal(t, vtypes.DepositStatusEscrowed, got.DepositStatus)
			}
		})
	}
}

func TestRemoveAttestationDisposition(t *testing.T) {
	bank := &recordingBank{}
	ctx, k := setupStoreKeeperWithOptions(t, WithBankKeeper(bank))
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	params := k.GetParams(ctx)
	attestation := attestationRecord(provider, auditor)
	require.NoError(t, k.SetAttestation(ctx, attestation))
	escrow := openAuditEscrowRecord(ctx, provider, attestation.AuditEscrowID, params)
	escrow.Status = vtypes.AuditEscrowStatusConsumed
	escrow.ConsumedByAuditor = auditor.String()
	now := ctx.BlockTime()
	escrow.ConsumedAt = &now
	require.NoError(t, k.SetAuditEscrow(ctx, escrow))

	err := k.RemoveAttestation(ctx, provider, auditor)
	require.NoError(t, err)

	got, found := k.GetAttestation(ctx, provider, auditor)
	require.True(t, found)
	require.Equal(t, vtypes.AttestationStatusRemoved, got.Status)
	require.Equal(t, vtypes.FaultAttributionNoFault, got.FaultAttribution)
	require.Equal(t, vtypes.FeeStatusReleasedToAuditor, got.FeeStatus)
	require.Equal(t, vtypes.DepositStatusReturnedToAuditor, got.DepositStatus)

	settledEscrow, found := k.GetAuditEscrow(ctx, attestation.AuditEscrowID)
	require.True(t, found)
	require.Equal(t, vtypes.AuditEscrowStatusSettled, settledEscrow.Status)
	require.Equal(t, vtypes.FeeStatusReleasedToAuditor, settledEscrow.FeeStatus)
	require.Equal(t, vtypes.ProviderDepositStatusReturnedToProvider, settledEscrow.ProviderDepositStatus)
	require.Equal(t, vtypes.AuditEscrowSettlementReasonNoFault, settledEscrow.SettlementReason)
	require.Equal(t, vtypes.FaultAttributionNoFault, settledEscrow.FaultAttribution)
	require.Equal(t, []bankTransfer{
		{to: auditor, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestation.Fee)},
		{to: auditor, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestation.Deposit)},
		{to: provider, module: moduletypes.ModuleName, amt: sdk.NewCoins(escrow.ProviderDeposit)},
	}, bank.moduleToAccount)
	testutil.EnsureEvent(t, ctx.EventManager().Events().ToABCIEvents(), &vtypes.EventAuditEscrowSettled{
		AuditEscrowID:    attestation.AuditEscrowID,
		Reason:           vtypes.AuditEscrowSettlementReasonNoFault,
		FaultAttribution: vtypes.FaultAttributionNoFault,
	})
}

func TestRemoveAttestationRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(sdk.Context, Keeper, sdk.AccAddress, sdk.AccAddress)
		provider  func(sdk.AccAddress) sdk.AccAddress
		auditor   func(sdk.AccAddress) sdk.AccAddress
		wantErr   error
		wantState vtypes.AttestationStatus
	}{
		{
			name: "wrong provider has no authorization over record",
			setup: func(ctx sdk.Context, k Keeper, provider sdk.AccAddress, auditor sdk.AccAddress) {
				require.NoError(t, k.SetAttestation(ctx, attestationRecord(provider, auditor)))
			},
			provider: func(sdk.AccAddress) sdk.AccAddress { return testutil.AccAddress(t) },
			auditor:  func(auditor sdk.AccAddress) sdk.AccAddress { return auditor },
			wantErr:  moduletypes.ErrAttestationNotFound,
		},
		{
			name: "non escrowed funds",
			setup: func(ctx sdk.Context, k Keeper, provider sdk.AccAddress, auditor sdk.AccAddress) {
				attestation := attestationRecord(provider, auditor)
				attestation.FeeStatus = vtypes.FeeStatusReleasedToAuditor
				require.NoError(t, k.SetAttestation(ctx, attestation))
			},
			provider:  func(provider sdk.AccAddress) sdk.AccAddress { return provider },
			auditor:   func(auditor sdk.AccAddress) sdk.AccAddress { return auditor },
			wantErr:   moduletypes.ErrInvalidReason,
			wantState: vtypes.AttestationStatusValid,
		},
		{
			name: "already revoked attestation",
			setup: func(ctx sdk.Context, k Keeper, provider sdk.AccAddress, auditor sdk.AccAddress) {
				attestation := attestationRecord(provider, auditor)
				attestation.Status = vtypes.AttestationStatusRevoked
				require.NoError(t, k.SetAttestation(ctx, attestation))
			},
			provider:  func(provider sdk.AccAddress) sdk.AccAddress { return provider },
			auditor:   func(auditor sdk.AccAddress) sdk.AccAddress { return auditor },
			wantErr:   moduletypes.ErrInvalidReason,
			wantState: vtypes.AttestationStatusRevoked,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bank := &recordingBank{}
			ctx, k := setupStoreKeeperWithOptions(t, WithBankKeeper(bank))
			provider := testutil.AccAddress(t)
			auditor := testutil.AccAddress(t)
			tc.setup(ctx, k, provider, auditor)

			err := k.RemoveAttestation(ctx, tc.provider(provider), tc.auditor(auditor))
			require.ErrorIs(t, err, tc.wantErr)
			require.Empty(t, bank.moduleToAccount)
			require.Empty(t, bank.moduleToModule)

			if tc.wantState != vtypes.AttestationStatusUnspecified {
				got, found := k.GetAttestation(ctx, provider, auditor)
				require.True(t, found)
				require.Equal(t, tc.wantState, got.Status)
			}
		})
	}
}

func TestRevokeProviderAttestationVoidsSingleAttestation(t *testing.T) {
	tests := []struct {
		name              string
		reason            vtypes.GovernanceAttestationReason
		fault             vtypes.FaultAttribution
		wantFeeStatus     vtypes.FeeStatus
		wantDepositStatus vtypes.DepositStatus
		wantAccountTxs    []func(sdk.AccAddress, sdk.AccAddress, vtypes.AttestationRecord) bankTransfer
		wantModuleTxs     []func(vtypes.AttestationRecord) bankTransfer
	}{
		{
			name:              "provider fault pays auditor",
			reason:            vtypes.GovernanceAttestationReasonFraudulentProvider,
			fault:             vtypes.FaultAttributionProviderFault,
			wantFeeStatus:     vtypes.FeeStatusReleasedToAuditor,
			wantDepositStatus: vtypes.DepositStatusReturnedToAuditor,
			wantAccountTxs: []func(sdk.AccAddress, sdk.AccAddress, vtypes.AttestationRecord) bankTransfer{
				func(_ sdk.AccAddress, auditor sdk.AccAddress, attestation vtypes.AttestationRecord) bankTransfer {
					return bankTransfer{to: auditor, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestation.Fee)}
				},
				func(_ sdk.AccAddress, auditor sdk.AccAddress, attestation vtypes.AttestationRecord) bankTransfer {
					return bankTransfer{to: auditor, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestation.Deposit)}
				},
			},
		},
		{
			name:              "auditor fault refunds provider and slashes deposit",
			reason:            vtypes.GovernanceAttestationReasonFaultyAuditor,
			fault:             vtypes.FaultAttributionAuditorFault,
			wantFeeStatus:     vtypes.FeeStatusReturnedToProvider,
			wantDepositStatus: vtypes.DepositStatusSlashed,
			wantAccountTxs: []func(sdk.AccAddress, sdk.AccAddress, vtypes.AttestationRecord) bankTransfer{
				func(provider sdk.AccAddress, _ sdk.AccAddress, attestation vtypes.AttestationRecord) bankTransfer {
					return bankTransfer{to: provider, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestation.Fee)}
				},
			},
			wantModuleTxs: []func(vtypes.AttestationRecord) bankTransfer{
				func(attestation vtypes.AttestationRecord) bankTransfer {
					return bankTransfer{fromModule: moduletypes.ModuleName, toModule: distrtypes.ModuleName, amt: sdk.NewCoins(attestation.Deposit)}
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bank := &recordingBank{}
			ctx, k := setupStoreKeeperWithOptions(t, WithAuthority("gov"), WithBankKeeper(bank))
			provider := testutil.AccAddress(t)
			auditor := testutil.AccAddress(t)
			attestation := attestationRecord(provider, auditor)
			require.NoError(t, k.SetAttestation(ctx, attestation))

			err := k.RevokeProviderAttestation(ctx, "gov", provider, auditor, tc.reason, tc.fault, testHash())
			require.NoError(t, err)

			got, found := k.GetAttestation(ctx, provider, auditor)
			require.True(t, found)
			require.Equal(t, vtypes.AttestationStatusVoided, got.Status)
			require.Equal(t, vtypes.VoidedReasonGovernance, got.VoidedReason)
			require.Equal(t, tc.fault, got.FaultAttribution)
			require.Equal(t, tc.wantFeeStatus, got.FeeStatus)
			require.Equal(t, tc.wantDepositStatus, got.DepositStatus)

			wantAccountTxs := make([]bankTransfer, 0, len(tc.wantAccountTxs))
			for _, build := range tc.wantAccountTxs {
				wantAccountTxs = append(wantAccountTxs, build(provider, auditor, attestation))
			}
			wantModuleTxs := make([]bankTransfer, 0, len(tc.wantModuleTxs))
			for _, build := range tc.wantModuleTxs {
				wantModuleTxs = append(wantModuleTxs, build(attestation))
			}
			require.Equal(t, wantAccountTxs, bank.moduleToAccount)
			if len(wantModuleTxs) == 0 {
				require.Empty(t, bank.moduleToModule)
			} else {
				require.Equal(t, wantModuleTxs, bank.moduleToModule)
			}
			testutil.EnsureEvent(t, ctx.EventManager().Events().ToABCIEvents(), &vtypes.EventAttestationVoided{
				Provider: provider.String(),
				Auditor:  auditor.String(),
				Reason:   vtypes.VoidedReasonGovernance,
			})
		})
	}
}

func TestRevokeAllProviderAttestationsVoidsActiveProviderAttestations(t *testing.T) {
	bank := &recordingBank{}
	ctx, k := setupStoreKeeperWithOptions(t, WithAuthority("gov"), WithBankKeeper(bank))
	provider := testutil.AccAddress(t)
	auditorA := testutil.AccAddress(t)
	auditorB := testutil.AccAddress(t)
	attestationA := attestationRecord(provider, auditorA)
	attestationB := attestationRecord(provider, auditorB)
	require.NoError(t, k.SetAttestation(ctx, attestationA))
	require.NoError(t, k.SetAttestation(ctx, attestationB))

	otherProvider := testutil.AccAddress(t)
	otherAttestation := attestationRecord(otherProvider, auditorA)
	require.NoError(t, k.SetAttestation(ctx, otherAttestation))

	err := k.RevokeAllProviderAttestations(
		ctx,
		"gov",
		provider,
		vtypes.GovernanceAttestationReasonFraudulentProvider,
		vtypes.FaultAttributionProviderFault,
		testHash(),
	)
	require.NoError(t, err)

	gotA, found := k.GetAttestation(ctx, provider, auditorA)
	require.True(t, found)
	require.Equal(t, vtypes.AttestationStatusVoided, gotA.Status)
	require.Equal(t, vtypes.VoidedReasonGovernance, gotA.VoidedReason)
	gotB, found := k.GetAttestation(ctx, provider, auditorB)
	require.True(t, found)
	require.Equal(t, vtypes.AttestationStatusVoided, gotB.Status)
	require.Equal(t, vtypes.VoidedReasonGovernance, gotB.VoidedReason)
	otherGot, found := k.GetAttestation(ctx, otherProvider, auditorA)
	require.True(t, found)
	require.Equal(t, vtypes.AttestationStatusValid, otherGot.Status)

	require.ElementsMatch(t, []bankTransfer{
		{to: auditorA, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestationA.Fee)},
		{to: auditorA, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestationA.Deposit)},
		{to: auditorB, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestationB.Fee)},
		{to: auditorB, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestationB.Deposit)},
	}, bank.moduleToAccount)
	require.Empty(t, bank.moduleToModule)
}

func TestRevokeAuditorAttestationsVoidsWorkAndSlashesBond(t *testing.T) {
	bank := &recordingBank{}
	ctx, k := setupStoreKeeperWithOptions(t, WithAuthority("gov"), WithBankKeeper(bank))
	auditor := testutil.AccAddress(t)
	otherAuditor := testutil.AccAddress(t)
	auditorRecord := auditorRecord(auditor)
	require.NoError(t, k.SetAuditor(ctx, auditorRecord))

	providerA := testutil.AccAddress(t)
	providerB := testutil.AccAddress(t)
	attestationA := attestationRecord(providerA, auditor)
	attestationB := attestationRecord(providerB, auditor)
	require.NoError(t, k.SetAttestation(ctx, attestationA))
	require.NoError(t, k.SetAttestation(ctx, attestationB))

	otherAttestation := attestationRecord(providerA, otherAuditor)
	require.NoError(t, k.SetAttestation(ctx, otherAttestation))

	err := k.RevokeAuditorAttestations(
		ctx,
		"gov",
		auditor,
		vtypes.GovernanceAttestationReasonFaultyAuditor,
		vtypes.FaultAttributionAuditorFault,
		testHash(),
	)
	require.NoError(t, err)

	gotA, found := k.GetAttestation(ctx, providerA, auditor)
	require.True(t, found)
	require.Equal(t, vtypes.AttestationStatusVoided, gotA.Status)
	require.Equal(t, vtypes.FeeStatusReturnedToProvider, gotA.FeeStatus)
	require.Equal(t, vtypes.DepositStatusSlashed, gotA.DepositStatus)
	gotB, found := k.GetAttestation(ctx, providerB, auditor)
	require.True(t, found)
	require.Equal(t, vtypes.AttestationStatusVoided, gotB.Status)
	require.Equal(t, vtypes.FeeStatusReturnedToProvider, gotB.FeeStatus)
	require.Equal(t, vtypes.DepositStatusSlashed, gotB.DepositStatus)
	otherGot, found := k.GetAttestation(ctx, providerA, otherAuditor)
	require.True(t, found)
	require.Equal(t, vtypes.AttestationStatusValid, otherGot.Status)

	gotAuditor, found := k.GetAuditor(ctx, auditor)
	require.True(t, found)
	require.True(t, gotAuditor.BondAmount.IsZero())
	require.Equal(t, vtypes.BondStatusNotBonded, gotAuditor.BondStatus)

	require.ElementsMatch(t, []bankTransfer{
		{to: providerA, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestationA.Fee)},
		{to: providerB, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestationB.Fee)},
	}, bank.moduleToAccount)
	require.ElementsMatch(t, []bankTransfer{
		{fromModule: moduletypes.ModuleName, toModule: distrtypes.ModuleName, amt: sdk.NewCoins(attestationA.Deposit)},
		{fromModule: moduletypes.ModuleName, toModule: distrtypes.ModuleName, amt: sdk.NewCoins(attestationB.Deposit)},
		{fromModule: moduletypes.ModuleName, toModule: distrtypes.ModuleName, amt: sdk.NewCoins(auditorRecord.BondAmount)},
	}, bank.moduleToModule)
}

func TestCancelAuditEscrowSettlesUnconsumedEscrow(t *testing.T) {
	bank := &recordingBank{}
	ctx, k := setupStoreKeeperWithOptions(t, WithBankKeeper(bank))
	provider := testutil.AccAddress(t)
	params := k.GetParams(ctx)

	require.NoError(t, k.SetAuditEscrow(ctx, openAuditEscrowRecord(ctx, provider, 1, params)))

	err := k.CancelAuditEscrow(ctx, provider, 1)
	require.NoError(t, err)

	escrow, found := k.GetAuditEscrow(ctx, 1)
	require.True(t, found)
	require.Equal(t, vtypes.AuditEscrowStatusCancelled, escrow.Status)
	require.Equal(t, vtypes.FeeStatusReturnedToProvider, escrow.FeeStatus)
	require.Equal(t, vtypes.ProviderDepositStatusReturnedToProvider, escrow.ProviderDepositStatus)
	require.Equal(t, vtypes.AuditEscrowSettlementReasonCancelledUnconsumed, escrow.SettlementReason)
	require.Equal(t, vtypes.FaultAttributionNoFault, escrow.FaultAttribution)
	require.Empty(t, escrow.ConsumedByAuditor)
	require.Nil(t, escrow.ConsumedAt)

	testutil.EnsureEvent(t, ctx.EventManager().Events().ToABCIEvents(), &vtypes.EventAuditEscrowSettled{
		AuditEscrowID:    1,
		Reason:           vtypes.AuditEscrowSettlementReasonCancelledUnconsumed,
		FaultAttribution: vtypes.FaultAttributionNoFault,
	})

	require.Equal(t, []bankTransfer{
		{to: provider, module: moduletypes.ModuleName, amt: sdk.NewCoins(params.MinFeeL1)},
		{to: provider, module: moduletypes.ModuleName, amt: sdk.NewCoins(params.ProviderAuditDeposit)},
	}, bank.moduleToAccount)
}

func TestCancelAuditEscrowRejectsWrongProvider(t *testing.T) {
	ctx, k := setupStoreKeeper(t)
	provider := testutil.AccAddress(t)
	params := k.GetParams(ctx)

	require.NoError(t, k.SetAuditEscrow(ctx, openAuditEscrowRecord(ctx, provider, 1, params)))

	err := k.CancelAuditEscrow(ctx, testutil.AccAddress(t), 1)
	require.ErrorIs(t, err, moduletypes.ErrUnauthorizedAuditEscrowSettlement)
}

func TestCancelAuditEscrowRejectsExpiredEscrow(t *testing.T) {
	ctx, k := setupStoreKeeper(t)
	provider := testutil.AccAddress(t)
	params := k.GetParams(ctx)
	escrow := openAuditEscrowRecord(ctx, provider, 1, params)
	escrow.ExpiresAt = ctx.BlockTime().Add(-time.Second)

	require.NoError(t, k.SetAuditEscrow(ctx, escrow))

	err := k.CancelAuditEscrow(ctx, provider, 1)
	require.ErrorIs(t, err, moduletypes.ErrAuditEscrowNotConsumable)
}

func TestCancelAuditEscrowRejectsConsumedEscrow(t *testing.T) {
	ctx, k := setupStoreKeeper(t)
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	params := k.GetParams(ctx)
	consumedAt := ctx.BlockTime()
	escrow := openAuditEscrowRecord(ctx, provider, 1, params)
	escrow.ConsumedByAuditor = auditor.String()
	escrow.ConsumedAt = &consumedAt

	require.NoError(t, k.SetAuditEscrow(ctx, escrow))

	err := k.CancelAuditEscrow(ctx, provider, 1)
	require.ErrorIs(t, err, moduletypes.ErrAuditEscrowNotConsumable)
}

func TestSettleAuditEscrowProviderFaultSlashesDeposit(t *testing.T) {
	bank := &recordingBank{}
	ctx, k := setupStoreKeeperWithOptions(t, WithAuthority("gov"), WithBankKeeper(bank))
	provider := testutil.AccAddress(t)
	params := k.GetParams(ctx)

	require.NoError(t, k.SetAuditEscrow(ctx, openAuditEscrowRecord(ctx, provider, 1, params)))

	err := k.SettleAuditEscrow(
		ctx,
		"gov",
		1,
		vtypes.AuditEscrowSettlementReasonProviderFault,
		vtypes.FaultAttributionProviderFault,
		testHash(),
	)
	require.NoError(t, err)

	escrow, found := k.GetAuditEscrow(ctx, 1)
	require.True(t, found)
	require.Equal(t, vtypes.AuditEscrowStatusSettled, escrow.Status)
	require.Equal(t, vtypes.FeeStatusReturnedToProvider, escrow.FeeStatus)
	require.Equal(t, vtypes.ProviderDepositStatusSlashed, escrow.ProviderDepositStatus)
	require.Equal(t, vtypes.AuditEscrowSettlementReasonProviderFault, escrow.SettlementReason)
	require.Equal(t, vtypes.FaultAttributionProviderFault, escrow.FaultAttribution)

	testutil.EnsureEvent(t, ctx.EventManager().Events().ToABCIEvents(), &vtypes.EventAuditEscrowSettled{
		AuditEscrowID:    1,
		Reason:           vtypes.AuditEscrowSettlementReasonProviderFault,
		FaultAttribution: vtypes.FaultAttributionProviderFault,
	})

	require.Equal(t, []bankTransfer{
		{to: provider, module: moduletypes.ModuleName, amt: sdk.NewCoins(params.MinFeeL1)},
	}, bank.moduleToAccount)
	require.Equal(t, []bankTransfer{
		{fromModule: moduletypes.ModuleName, toModule: distrtypes.ModuleName, amt: sdk.NewCoins(params.ProviderAuditDeposit)},
	}, bank.moduleToModule)
}

func TestSettleAuditEscrowRejectsInvalidReasonAttribution(t *testing.T) {
	ctx, k := setupStoreKeeperWithOptions(t, WithAuthority("gov"))
	provider := testutil.AccAddress(t)
	params := k.GetParams(ctx)

	require.NoError(t, k.SetAuditEscrow(ctx, openAuditEscrowRecord(ctx, provider, 1, params)))

	err := k.SettleAuditEscrow(
		ctx,
		"gov",
		1,
		vtypes.AuditEscrowSettlementReasonProviderFault,
		vtypes.FaultAttributionNoFault,
		testHash(),
	)
	require.ErrorIs(t, err, moduletypes.ErrInvalidFaultAttribution)
}

func TestSettleAuditEscrowRejectsMalformedEvidenceHash(t *testing.T) {
	ctx, k := setupStoreKeeperWithOptions(t, WithAuthority("gov"))
	provider := testutil.AccAddress(t)
	params := k.GetParams(ctx)

	require.NoError(t, k.SetAuditEscrow(ctx, openAuditEscrowRecord(ctx, provider, 1, params)))

	err := k.SettleAuditEscrow(
		ctx,
		"gov",
		1,
		vtypes.AuditEscrowSettlementReasonNoFault,
		vtypes.FaultAttributionNoFault,
		[]byte("short"),
	)
	require.ErrorIs(t, err, moduletypes.ErrInvalidReason)
}

func TestSettleAuditEscrowRejectsNonGovernanceSettlementReason(t *testing.T) {
	ctx, k := setupStoreKeeperWithOptions(t, WithAuthority("gov"))
	provider := testutil.AccAddress(t)
	params := k.GetParams(ctx)

	require.NoError(t, k.SetAuditEscrow(ctx, openAuditEscrowRecord(ctx, provider, 1, params)))

	tests := []vtypes.AuditEscrowSettlementReason{
		vtypes.AuditEscrowSettlementReasonCancelledUnconsumed,
		vtypes.AuditEscrowSettlementReasonExpiredUnconsumed,
	}

	for _, reason := range tests {
		t.Run(reason.String(), func(t *testing.T) {
			err := k.SettleAuditEscrow(
				ctx,
				"gov",
				1,
				reason,
				vtypes.FaultAttributionNoFault,
				testHash(),
			)
			require.ErrorIs(t, err, moduletypes.ErrInvalidReason)
		})
	}
}

func TestWithdrawProviderBondVoidsUnsupportedAttestations(t *testing.T) {
	tests := []struct {
		name       string
		amount     sdk.Coin
		wantStatus vtypes.AttestationStatus
		wantReason vtypes.VoidedReason
	}{
		{
			name:       "leaves current tier minimum bonded",
			amount:     sdk.NewInt64Coin(bondDenom, 95000000),
			wantStatus: vtypes.AttestationStatusValid,
		},
		{
			name:       "voids attestation below current tier minimum",
			amount:     sdk.NewInt64Coin(bondDenom, 100000000),
			wantStatus: vtypes.AttestationStatusVoided,
			wantReason: vtypes.VoidedReasonBondWithdrawn,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bank := &recordingBank{}
			ctx, k := setupStoreKeeperWithOptions(t, WithBankKeeper(bank))
			provider := testutil.AccAddress(t)
			auditor := testutil.AccAddress(t)
			attestation := attestationRecord(provider, auditor)
			require.NoError(t, k.SetProviderBond(ctx, providerBondRecord(provider)))
			require.NoError(t, k.SetProviderSnapshot(ctx, providerSnapshotRecord(provider)))
			require.NoError(t, k.SetAttestation(ctx, attestation))

			err := k.WithdrawProviderBond(ctx, provider, tc.amount)
			require.NoError(t, err)

			bond, found := k.GetProviderBond(ctx, provider)
			require.True(t, found)
			require.Equal(t, sdk.NewCoin(bondDenom, providerBondRecord(provider).BondedAmount.Amount.Sub(tc.amount.Amount)), bond.BondedAmount)
			require.Len(t, bond.UnbondingEntries, 1)
			require.Equal(t, tc.amount, bond.UnbondingEntries[0].Amount)
			require.Equal(t, ctx.BlockTime().Add(k.GetParams(ctx).ProviderBondUnbondingPeriod), bond.UnbondingEntries[0].CompletionTime)

			got, found := k.GetAttestation(ctx, provider, auditor)
			require.True(t, found)
			require.Equal(t, tc.wantStatus, got.Status)
			require.Equal(t, tc.wantReason, got.VoidedReason)
			if tc.wantStatus == vtypes.AttestationStatusVoided {
				require.Equal(t, vtypes.FeeStatusReleasedToAuditor, got.FeeStatus)
				require.Equal(t, vtypes.DepositStatusReturnedToAuditor, got.DepositStatus)
				require.Equal(t, []bankTransfer{
					{to: auditor, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestation.Fee)},
					{to: auditor, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestation.Deposit)},
				}, bank.moduleToAccount)
			}
		})
	}
}

func TestProviderBondUnbondingCompletionMovesFunds(t *testing.T) {
	bank := &recordingBank{}
	ctx, k := setupStoreKeeperWithOptions(t, WithBankKeeper(bank))
	provider := testutil.AccAddress(t)
	amount := sdk.NewInt64Coin(bondDenom, 50000000)
	require.NoError(t, k.SetProviderBond(ctx, providerBondRecord(provider)))
	require.NoError(t, k.WithdrawProviderBond(ctx, provider, amount))

	ctx = ctx.WithBlockTime(ctx.BlockTime().Add(k.GetParams(ctx).ProviderBondUnbondingPeriod))
	require.NoError(t, k.EndBlocker(ctx))

	bond, found := k.GetProviderBond(ctx, provider)
	require.True(t, found)
	require.Empty(t, bond.UnbondingEntries)
	require.Equal(t, []bankTransfer{
		{to: provider, module: moduletypes.ModuleName, amt: sdk.NewCoins(amount)},
	}, bank.moduleToAccount)
}

func TestSlashProviderBondValidation(t *testing.T) {
	tests := []struct {
		name     string
		fraction sdkmath.LegacyDec
		reason   vtypes.ProviderBondSlashReason
		evidence []byte
		wantErr  error
	}{
		{
			name:     "rejects unspecified reason",
			fraction: sdkmath.LegacyMustNewDecFromStr("0.5"),
			reason:   vtypes.ProviderBondSlashReasonUnspecified,
			evidence: testHash(),
			wantErr:  moduletypes.ErrInvalidReason,
		},
		{
			name:     "rejects zero fraction",
			fraction: sdkmath.LegacyZeroDec(),
			reason:   vtypes.ProviderBondSlashReasonFraudulentSnapshot,
			evidence: testHash(),
			wantErr:  moduletypes.ErrInvalidReason,
		},
		{
			name:     "rejects fraction above one",
			fraction: sdkmath.LegacyMustNewDecFromStr("1.1"),
			reason:   vtypes.ProviderBondSlashReasonFraudulentSnapshot,
			evidence: testHash(),
			wantErr:  moduletypes.ErrInvalidReason,
		},
		{
			name:     "rejects malformed evidence hash",
			fraction: sdkmath.LegacyMustNewDecFromStr("0.5"),
			reason:   vtypes.ProviderBondSlashReasonFraudulentSnapshot,
			evidence: []byte("short"),
			wantErr:  moduletypes.ErrInvalidReason,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, k := setupStoreKeeperWithOptions(t, WithAuthority("gov"))
			provider := testutil.AccAddress(t)
			require.NoError(t, k.SetProviderBond(ctx, providerBondRecord(provider)))

			err := k.SlashProviderBond(ctx, "gov", provider, tc.fraction, tc.reason, tc.evidence)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestSlashProviderBondLeavesSupportedAttestationsValid(t *testing.T) {
	bank := &recordingBank{}
	ctx, k := setupStoreKeeperWithOptions(t, WithAuthority("gov"), WithBankKeeper(bank))
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	require.NoError(t, k.SetProviderBond(ctx, providerBondRecord(provider)))
	require.NoError(t, k.SetProviderSnapshot(ctx, providerSnapshotRecord(provider)))
	require.NoError(t, k.SetAttestation(ctx, attestationRecord(provider, auditor)))

	err := k.SlashProviderBond(
		ctx,
		"gov",
		provider,
		sdkmath.LegacyMustNewDecFromStr("0.1"),
		vtypes.ProviderBondSlashReasonFraudulentSnapshot,
		testHash(),
	)
	require.NoError(t, err)

	attestation, found := k.GetAttestation(ctx, provider, auditor)
	require.True(t, found)
	require.Equal(t, vtypes.AttestationStatusValid, attestation.Status)
	require.Empty(t, bank.moduleToAccount)
}

func TestSlashProviderBondSlashesFundsAndVoidsAttestations(t *testing.T) {
	bank := &recordingBank{}
	ctx, k := setupStoreKeeperWithOptions(t, WithAuthority("gov"), WithBankKeeper(bank))
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	require.NoError(t, k.SetProviderBond(ctx, providerBondRecord(provider)))
	require.NoError(t, k.SetProviderSnapshot(ctx, providerSnapshotRecord(provider)))
	require.NoError(t, k.SetAttestation(ctx, attestationRecord(provider, auditor)))

	err := k.SlashProviderBond(
		ctx,
		"gov",
		provider,
		sdkmath.LegacyMustNewDecFromStr("0.5"),
		vtypes.ProviderBondSlashReasonFraudulentSnapshot,
		testHash(),
	)
	require.NoError(t, err)

	bond, found := k.GetProviderBond(ctx, provider)
	require.True(t, found)
	require.Equal(t, sdk.NewInt64Coin(bondDenom, 100000000), bond.BondedAmount)
	require.True(t, bond.Slashed)
	require.NotNil(t, bond.LastSlashTime)
	require.Equal(t, ctx.BlockTime(), *bond.LastSlashTime)

	attestation, found := k.GetAttestation(ctx, provider, auditor)
	require.True(t, found)
	require.Equal(t, vtypes.AttestationStatusVoided, attestation.Status)
	require.Equal(t, vtypes.VoidedReasonBondSlashed, attestation.VoidedReason)
	require.Equal(t, vtypes.FeeStatusReleasedToAuditor, attestation.FeeStatus)
	require.Equal(t, vtypes.DepositStatusReturnedToAuditor, attestation.DepositStatus)
	require.Equal(t, vtypes.FaultAttributionProviderFault, attestation.FaultAttribution)
	require.Equal(t, []bankTransfer{
		{to: auditor, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestationRecord(provider, auditor).Fee)},
		{to: auditor, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestationRecord(provider, auditor).Deposit)},
	}, bank.moduleToAccount)
	require.Equal(t, []bankTransfer{
		{fromModule: moduletypes.ModuleName, toModule: distrtypes.ModuleName, amt: sdk.NewCoins(sdk.NewInt64Coin(bondDenom, 100000000))},
	}, bank.moduleToModule)
}

func TestSubmitAttestationRejectsMalformedEvidenceHash(t *testing.T) {
	ctx, k := setupStoreKeeper(t)
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	params := k.GetParams(ctx)

	err := k.SubmitAttestation(
		ctx,
		provider,
		auditor,
		vtypes.TierIdentified,
		nil,
		[]byte("short"),
		params.MinFeeL1,
		params.AttestationDeposit,
		1,
	)
	require.ErrorIs(t, err, moduletypes.ErrInvalidReason)
}

func TestSubmitAttestationRejectsInvalidCapabilities(t *testing.T) {
	ctx, k := setupStoreKeeper(t)
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	params := k.GetParams(ctx)

	err := k.SubmitAttestation(
		ctx,
		provider,
		auditor,
		vtypes.TierIdentified,
		[]vtypes.CapabilityFlag{vtypes.CapabilityUnspecified},
		testHash(),
		params.MinFeeL1,
		params.AttestationDeposit,
		1,
	)
	require.ErrorIs(t, err, moduletypes.ErrInvalidReason)
}

func TestPostSnapshotHashRejectsMalformedHash(t *testing.T) {
	ctx, k := setupStoreKeeper(t)

	err := k.PostSnapshotHash(ctx, testutil.AccAddress(t), []byte("short"), vtypes.ResourceSummary{}, ctx.BlockTime())
	require.ErrorIs(t, err, moduletypes.ErrInvalidReason)
}

func TestOpenAuditEscrowRejectsInvalidCapabilities(t *testing.T) {
	ctx, k := setupStoreKeeper(t)
	params := k.GetParams(ctx)

	_, err := k.OpenAuditEscrow(
		ctx,
		testutil.AccAddress(t),
		vtypes.TierIdentified,
		[]vtypes.CapabilityFlag{vtypes.CapabilityBareMetal, vtypes.CapabilityBareMetal},
		params.MinFeeL1,
		params.ProviderAuditDeposit,
		ctx.BlockTime().Add(params.TtlL1),
		nil,
	)
	require.ErrorIs(t, err, moduletypes.ErrInvalidReason)
}

func TestRegisterAuditorRejectsWrongAuthority(t *testing.T) {
	ctx, k := setupStoreKeeperWithOptions(t, WithAuthority("gov"))

	err := k.RegisterAuditor(ctx, "not-gov", testutil.AccAddress(t), vtypes.TierIdentified, nil)
	require.ErrorIs(t, err, moduletypes.ErrAuditorUnauthorizedTier)
}

func TestRenewAuditorResetsDeadline(t *testing.T) {
	ctx, k := setupStoreKeeperWithOptions(t, WithAuthority("gov"))
	auditor := testutil.AccAddress(t)
	params := k.GetParams(ctx)
	record := auditorRecord(auditor)
	record.Status = vtypes.AuditorStatusLapsed
	record.MaxAttestationTier = vtypes.TierEstablished
	record.RenewalDeadline = ctx.BlockTime().Add(-time.Hour)
	require.NoError(t, k.SetAuditor(ctx, record))

	err := k.RenewAuditor(ctx, "not-gov", auditor)
	require.ErrorIs(t, err, moduletypes.ErrAuditorUnauthorizedTier)

	err = k.RenewAuditor(ctx, "gov", auditor)
	require.NoError(t, err)

	got, found := k.GetAuditor(ctx, auditor)
	require.True(t, found)
	require.Equal(t, vtypes.AuditorStatusActive, got.Status)
	require.Equal(t, ctx.BlockTime().Add(params.RenewalPeriodL3), got.RenewalDeadline)
}

func TestRemoveAuditorStartsBondUnbonding(t *testing.T) {
	ctx, k := setupStoreKeeperWithOptions(t, WithAuthority("gov"))
	auditor := testutil.AccAddress(t)
	params := k.GetParams(ctx)
	record := auditorRecord(auditor)
	require.NoError(t, k.SetAuditor(ctx, record))

	err := k.RemoveAuditor(ctx, "not-gov", auditor)
	require.ErrorIs(t, err, moduletypes.ErrAuditorUnauthorizedTier)

	err = k.RemoveAuditor(ctx, "gov", auditor)
	require.NoError(t, err)

	got, found := k.GetAuditor(ctx, auditor)
	require.True(t, found)
	require.Equal(t, vtypes.AuditorStatusRemoved, got.Status)
	require.True(t, got.RenewalDeadline.IsZero())
	require.Equal(t, vtypes.BondStatusUnbonding, got.BondStatus)
	require.NotNil(t, got.BondUnbondingCompletionTime)
	require.Equal(t, ctx.BlockTime().Add(params.AuditorUnbondingPeriod), *got.BondUnbondingCompletionTime)
}

func TestResignAuditorStartsBondUnbonding(t *testing.T) {
	ctx, k := setupStoreKeeper(t)
	auditor := testutil.AccAddress(t)
	params := k.GetParams(ctx)
	record := auditorRecord(auditor)
	require.NoError(t, k.SetAuditor(ctx, record))

	err := k.ResignAuditor(ctx, auditor)
	require.NoError(t, err)

	got, found := k.GetAuditor(ctx, auditor)
	require.True(t, found)
	require.Equal(t, vtypes.AuditorStatusResigned, got.Status)
	require.True(t, got.RenewalDeadline.IsZero())
	require.Equal(t, vtypes.BondStatusUnbonding, got.BondStatus)
	require.NotNil(t, got.BondUnbondingCompletionTime)
	require.Equal(t, ctx.BlockTime().Add(params.AuditorUnbondingPeriod), *got.BondUnbondingCompletionTime)
}

func TestAuditorLifecycleRejectsFrozenBond(t *testing.T) {
	ctx, k := setupStoreKeeperWithOptions(t, WithAuthority("gov"))
	auditor := testutil.AccAddress(t)
	record := auditorRecord(auditor)
	record.BondStatus = vtypes.BondStatusFrozen
	require.NoError(t, k.SetAuditor(ctx, record))

	err := k.ResignAuditor(ctx, auditor)
	require.ErrorIs(t, err, moduletypes.ErrAuditorFrozen)

	err = k.RemoveAuditor(ctx, "gov", auditor)
	require.ErrorIs(t, err, moduletypes.ErrAuditorFrozen)
}

func testHash() []byte {
	return []byte("12345678901234567890123456789012")
}

func openAuditEscrowRecord(ctx sdk.Context, provider sdk.AccAddress, id uint64, params vtypes.Params) vtypes.AuditEscrowRecord {
	return vtypes.AuditEscrowRecord{
		ID:                    id,
		Provider:              provider.String(),
		RequestedTier:         vtypes.TierIdentified,
		Fee:                   params.MinFeeL1,
		FeeStatus:             vtypes.FeeStatusEscrowed,
		ProviderDeposit:       params.ProviderAuditDeposit,
		ProviderDepositStatus: vtypes.ProviderDepositStatusEscrowed,
		Status:                vtypes.AuditEscrowStatusOpen,
		OpenedAt:              ctx.BlockTime(),
		ExpiresAt:             ctx.BlockTime().Add(params.TtlL1),
	}
}

func setupL2AttestationPrerequisites(t testing.TB, registeredAt func(sdk.Context, vtypes.Params) time.Time) (sdk.Context, Keeper, sdk.AccAddress, sdk.AccAddress, vtypes.Params) {
	t.Helper()

	provider := testutil.AccAddress(t)
	providerKeeper := newStubProviderKeeper(provider)
	ctx, k := setupStoreKeeperWithOptions(t, WithProviderKeeper(providerKeeper))
	auditor := testutil.AccAddress(t)
	params := k.GetParams(ctx)
	providerKeeper.registrations[provider.String()] = registeredAt(ctx, params)

	require.NoError(t, k.SetAuditor(ctx, vtypes.AuditorRecord{
		Address:            auditor.String(),
		Status:             vtypes.AuditorStatusActive,
		MaxAttestationTier: vtypes.TierTrusted,
		BondAmount:         params.BondL4,
		BondStatus:         vtypes.BondStatusBonded,
		RegisteredAt:       ctx.BlockTime(),
		RenewalDeadline:    ctx.BlockTime().Add(params.RenewalPeriodL4),
	}))
	require.NoError(t, k.SetProviderBond(ctx, providerBondRecord(provider)))
	require.NoError(t, k.SetProviderSnapshot(ctx, providerSnapshotRecord(provider)))
	require.NoError(t, k.SetAuditEscrow(ctx, vtypes.AuditEscrowRecord{
		ID:                    1,
		Provider:              provider.String(),
		RequestedTier:         vtypes.TierVerified,
		Fee:                   params.MinFeeL2,
		FeeStatus:             vtypes.FeeStatusEscrowed,
		ProviderDeposit:       params.ProviderAuditDeposit,
		ProviderDepositStatus: vtypes.ProviderDepositStatusEscrowed,
		Status:                vtypes.AuditEscrowStatusOpen,
		OpenedAt:              ctx.BlockTime(),
		ExpiresAt:             ctx.BlockTime().Add(params.TtlL2),
	}))

	return ctx, k, provider, auditor, params
}

func setupL3AttestationPrerequisites(t testing.TB, market MarketStatsKeeper) (sdk.Context, Keeper, sdk.AccAddress, sdk.AccAddress, vtypes.Params) {
	t.Helper()

	provider := testutil.AccAddress(t)
	providerKeeper := newStubProviderKeeper(provider)
	ctx, k := setupStoreKeeperWithOptions(t, WithProviderKeeper(providerKeeper), WithMarketStatsKeeper(market))
	auditor := testutil.AccAddress(t)
	params := k.GetParams(ctx)
	providerKeeper.registrations[provider.String()] = ctx.BlockTime().Add(-params.MinAgeL3)

	require.NoError(t, k.SetAuditor(ctx, vtypes.AuditorRecord{
		Address:            auditor.String(),
		Status:             vtypes.AuditorStatusActive,
		MaxAttestationTier: vtypes.TierTrusted,
		BondAmount:         params.BondL4,
		BondStatus:         vtypes.BondStatusBonded,
		RegisteredAt:       ctx.BlockTime(),
		RenewalDeadline:    ctx.BlockTime().Add(params.RenewalPeriodL4),
	}))
	require.NoError(t, k.SetProviderBond(ctx, vtypes.ProviderBondRecord{
		Provider:     provider.String(),
		BondedAmount: params.BondL4,
	}))
	require.NoError(t, k.SetProviderSnapshot(ctx, providerSnapshotRecord(provider)))
	require.NoError(t, k.SetAuditEscrow(ctx, vtypes.AuditEscrowRecord{
		ID:                    1,
		Provider:              provider.String(),
		RequestedTier:         vtypes.TierEstablished,
		Fee:                   params.MinFeeL3,
		FeeStatus:             vtypes.FeeStatusEscrowed,
		ProviderDeposit:       params.ProviderAuditDeposit,
		ProviderDepositStatus: vtypes.ProviderDepositStatusEscrowed,
		Status:                vtypes.AuditEscrowStatusOpen,
		OpenedAt:              ctx.BlockTime(),
		ExpiresAt:             ctx.BlockTime().Add(params.TtlL3),
	}))

	return ctx, k, provider, auditor, params
}

func setupL4AttestationPrerequisites(t testing.TB, continuousHistory bool) (sdk.Context, Keeper, sdk.AccAddress, sdk.AccAddress, vtypes.Params) {
	t.Helper()

	provider := testutil.AccAddress(t)
	providerKeeper := newStubProviderKeeper(provider)
	ctx, k := setupStoreKeeperWithOptions(t,
		WithProviderKeeper(providerKeeper),
		WithMarketStatsKeeper(stubMarketStatsKeeper{
			completed: 98,
			failures: map[mv1.LeaseClosedReason]uint64{
				mv1.LeaseClosedReasonUnstable: 2,
			},
			found: true,
		}),
	)
	auditor := testutil.AccAddress(t)
	params := k.GetParams(ctx)
	providerKeeper.registrations[provider.String()] = ctx.BlockTime().Add(-params.MinAgeL4)

	require.NoError(t, k.SetAuditor(ctx, vtypes.AuditorRecord{
		Address:            auditor.String(),
		Status:             vtypes.AuditorStatusActive,
		MaxAttestationTier: vtypes.TierTrusted,
		BondAmount:         params.BondL4,
		BondStatus:         vtypes.BondStatusBonded,
		RegisteredAt:       ctx.BlockTime(),
		RenewalDeadline:    ctx.BlockTime().Add(params.RenewalPeriodL4),
	}))
	require.NoError(t, k.SetProviderBond(ctx, vtypes.ProviderBondRecord{
		Provider:     provider.String(),
		BondedAmount: params.BondL4,
	}))
	require.NoError(t, k.SetProviderSnapshot(ctx, providerSnapshotRecord(provider)))
	require.NoError(t, k.SetAuditEscrow(ctx, vtypes.AuditEscrowRecord{
		ID:                    1,
		Provider:              provider.String(),
		RequestedTier:         vtypes.TierTrusted,
		Fee:                   params.MinFeeL4,
		FeeStatus:             vtypes.FeeStatusEscrowed,
		ProviderDeposit:       params.ProviderAuditDeposit,
		ProviderDepositStatus: vtypes.ProviderDepositStatusEscrowed,
		Status:                vtypes.AuditEscrowStatusOpen,
		OpenedAt:              ctx.BlockTime(),
		ExpiresAt:             ctx.BlockTime().Add(params.TtlL4),
	}))

	historyStart := ctx.BlockTime().Add(-params.MinL3DurationForL4)
	firstAuditor := testutil.AccAddress(t)
	secondAuditor := testutil.AccAddress(t)
	first := attestationRecord(provider, firstAuditor)
	first.Tier = vtypes.TierEstablished
	first.Status = vtypes.AttestationStatusExpired
	first.CreatedAt = historyStart
	first.ExpiresAt = historyStart.Add(params.MinL3DurationForL4 / 2)
	require.NoError(t, k.SetAttestation(ctx, first))

	second := attestationRecord(provider, secondAuditor)
	second.Tier = vtypes.TierEstablished
	second.Status = vtypes.AttestationStatusValid
	second.CreatedAt = first.ExpiresAt
	if !continuousHistory {
		second.CreatedAt = second.CreatedAt.Add(time.Hour)
	}
	second.ExpiresAt = ctx.BlockTime().Add(time.Hour)
	require.NoError(t, k.SetAttestation(ctx, second))

	return ctx, k, provider, auditor, params
}

type stubMarketStatsKeeper struct {
	completed uint64
	failures  map[mv1.LeaseClosedReason]uint64
	found     bool
}

func (k stubMarketStatsKeeper) GetProviderLeaseStats(_ sdk.Context, _ sdk.Address) (uint64, map[mv1.LeaseClosedReason]uint64, bool) {
	return k.completed, k.failures, k.found
}

type stubProviderKeeper struct {
	providers     map[string]ptypes.Provider
	registrations map[string]time.Time
}

func newStubProviderKeeper(provider sdk.AccAddress) *stubProviderKeeper {
	return &stubProviderKeeper{
		providers: map[string]ptypes.Provider{
			provider.String(): {Owner: provider.String()},
		},
		registrations: make(map[string]time.Time),
	}
}

func (k *stubProviderKeeper) Get(_ sdk.Context, id sdk.Address) (ptypes.Provider, bool) {
	provider, found := k.providers[id.String()]
	return provider, found
}

func (k *stubProviderKeeper) GetRegistrationTime(_ sdk.Context, id sdk.Address) (time.Time, bool) {
	registeredAt, found := k.registrations[id.String()]
	return registeredAt, found
}

type recordingBank struct {
	accountToModule []bankTransfer
	moduleToAccount []bankTransfer
	moduleToModule  []bankTransfer
	balances        map[string]sdk.Coin
}

type bankTransfer struct {
	from       sdk.AccAddress
	to         sdk.AccAddress
	module     string
	fromModule string
	toModule   string
	amt        sdk.Coins
}

func (b *recordingBank) GetBalance(_ context.Context, _ sdk.AccAddress, denom string) sdk.Coin {
	if b.balances != nil {
		if balance, found := b.balances[denom]; found {
			return balance
		}
	}
	return sdk.NewInt64Coin(denom, 0)
}

func (b *recordingBank) SendCoinsFromAccountToModule(_ context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error {
	b.accountToModule = append(b.accountToModule, bankTransfer{
		from:   senderAddr,
		module: recipientModule,
		amt:    amt,
	})
	return nil
}

func (b *recordingBank) SendCoinsFromModuleToAccount(_ context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error {
	b.moduleToAccount = append(b.moduleToAccount, bankTransfer{
		to:     recipientAddr,
		module: senderModule,
		amt:    amt,
	})
	return nil
}

func (b *recordingBank) SendCoinsFromModuleToModule(_ context.Context, senderModule, recipientModule string, amt sdk.Coins) error {
	b.moduleToModule = append(b.moduleToModule, bankTransfer{
		fromModule: senderModule,
		toModule:   recipientModule,
		amt:        amt,
	})
	return nil
}
