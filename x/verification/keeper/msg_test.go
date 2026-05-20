package keeper

import (
	"context"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	vtypes "pkg.akt.dev/go/node/verification/v1"
	"pkg.akt.dev/go/testutil"

	moduletypes "pkg.akt.dev/node/v2/x/verification/types"
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

	require.Len(t, bank.accountToModule, 5)
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
	attestation := attestationRecord(provider, auditor)
	require.NoError(t, k.SetAttestation(ctx, attestation))

	err := k.RemoveAttestation(ctx, provider, auditor)
	require.NoError(t, err)

	got, found := k.GetAttestation(ctx, provider, auditor)
	require.True(t, found)
	require.Equal(t, vtypes.AttestationStatusRemoved, got.Status)
	require.Equal(t, vtypes.FaultAttributionNoFault, got.FaultAttribution)
	require.Equal(t, vtypes.FeeStatusReleasedToAuditor, got.FeeStatus)
	require.Equal(t, vtypes.DepositStatusReturnedToAuditor, got.DepositStatus)
	require.Equal(t, []bankTransfer{
		{to: auditor, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestation.Fee)},
		{to: auditor, module: moduletypes.ModuleName, amt: sdk.NewCoins(attestation.Deposit)},
	}, bank.moduleToAccount)
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

func TestWithdrawProviderBondMaintainsMinimum(t *testing.T) {
	tests := []struct {
		name    string
		amount  sdk.Coin
		wantErr error
	}{
		{
			name:   "leaves current tier minimum bonded",
			amount: sdk.NewInt64Coin(bondDenom, 95000000),
		},
		{
			name:    "rejects below current tier minimum",
			amount:  sdk.NewInt64Coin(bondDenom, 100000000),
			wantErr: moduletypes.ErrBondWithdrawalExceedsMinimum,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, k := setupStoreKeeper(t)
			provider := testutil.AccAddress(t)
			auditor := testutil.AccAddress(t)
			require.NoError(t, k.SetProviderBond(ctx, providerBondRecord(provider)))
			require.NoError(t, k.SetProviderSnapshot(ctx, providerSnapshotRecord(provider)))
			require.NoError(t, k.SetAttestation(ctx, attestationRecord(provider, auditor)))

			err := k.WithdrawProviderBond(ctx, provider, tc.amount)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)

			bond, found := k.GetProviderBond(ctx, provider)
			require.True(t, found)
			require.Equal(t, sdk.NewInt64Coin(bondDenom, 105000000), bond.BondedAmount)
			require.Len(t, bond.UnbondingEntries, 1)
			require.Equal(t, tc.amount, bond.UnbondingEntries[0].Amount)
			require.Equal(t, ctx.BlockTime().Add(k.GetParams(ctx).ProviderBondUnbondingPeriod), bond.UnbondingEntries[0].CompletionTime)
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

func TestSlashProviderBondSlashesFundsAndVoidsAttestations(t *testing.T) {
	bank := &recordingBank{}
	ctx, k := setupStoreKeeperWithOptions(t, WithAuthority("gov"), WithBankKeeper(bank))
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	require.NoError(t, k.SetProviderBond(ctx, providerBondRecord(provider)))
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
}

type bankTransfer struct {
	from       sdk.AccAddress
	to         sdk.AccAddress
	module     string
	fromModule string
	toModule   string
	amt        sdk.Coins
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
