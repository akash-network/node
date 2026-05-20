package keeper

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

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

func testHash() []byte {
	return []byte("12345678901234567890123456789012")
}

type recordingBank struct {
	accountToModule []bankTransfer
}

type bankTransfer struct {
	from   sdk.AccAddress
	module string
	amt    sdk.Coins
}

func (b *recordingBank) SendCoinsFromAccountToModule(_ context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error {
	b.accountToModule = append(b.accountToModule, bankTransfer{
		from:   senderAddr,
		module: recipientModule,
		amt:    amt,
	})
	return nil
}

func (b *recordingBank) SendCoinsFromModuleToAccount(context.Context, string, sdk.AccAddress, sdk.Coins) error {
	return nil
}
