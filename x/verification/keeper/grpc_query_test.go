package keeper

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	testutilmod "github.com/cosmos/cosmos-sdk/types/module/testutil"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	vtypes "pkg.akt.dev/go/node/verification/v1"
	"pkg.akt.dev/go/testutil"
)

func TestGRPCQueries(t *testing.T) {
	ctx, k := setupStoreKeeper(t)
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	auditorB := testutil.AccAddress(t)

	require.NoError(t, k.SetAuditor(ctx, auditorRecord(auditor)))
	require.NoError(t, k.SetAttestation(ctx, attestationRecord(provider, auditor)))
	require.NoError(t, k.SetProviderBond(ctx, providerBondRecord(provider)))
	require.NoError(t, k.SetProviderSnapshot(ctx, providerSnapshotRecord(provider)))
	require.NoError(t, k.SetAuditEscrow(ctx, auditEscrowRecord(provider, auditor)))
	require.NoError(t, k.SetProviderVerificationGrace(ctx, graceRecord(provider)))
	k.SetDiscrepancy(ctx, discrepancyRecord(provider, auditor, auditorB))

	client := queryClient(t, ctx, k)

	auditorRes, err := client.Auditor(ctx, &vtypes.QueryAuditorRequest{Auditor: auditor.String()})
	require.NoError(t, err)
	require.Equal(t, auditor.String(), auditorRes.Auditor.Address)

	auditorsRes, err := client.Auditors(ctx, &vtypes.QueryAuditorsRequest{
		StatusFilter: vtypes.AuditorStatusActive,
		Pagination:   &sdkquery.PageRequest{Limit: 1},
	})
	require.NoError(t, err)
	require.Len(t, auditorsRes.Auditors, 1)

	attestationRes, err := client.Attestation(ctx, &vtypes.QueryAttestationRequest{
		Provider: provider.String(),
		Auditor:  auditor.String(),
	})
	require.NoError(t, err)
	require.Equal(t, vtypes.TierVerified, attestationRes.Attestation.Tier)

	providerAttestationsRes, err := client.ProviderAttestations(ctx, &vtypes.QueryProviderAttestationsRequest{
		Provider:     provider.String(),
		StatusFilter: vtypes.AttestationStatusValid,
	})
	require.NoError(t, err)
	require.Len(t, providerAttestationsRes.Attestations, 1)

	auditorAttestationsRes, err := client.AuditorAttestations(ctx, &vtypes.QueryAuditorAttestationsRequest{
		Auditor: auditor.String(),
	})
	require.NoError(t, err)
	require.Len(t, auditorAttestationsRes.Attestations, 1)

	discrepancyRes, err := client.Discrepancy(ctx, &vtypes.QueryDiscrepancyRequest{Id: 11})
	require.NoError(t, err)
	require.Equal(t, provider.String(), discrepancyRes.Discrepancy.Provider)

	discrepanciesRes, err := client.Discrepancies(ctx, &vtypes.QueryDiscrepanciesRequest{
		StatusFilter: vtypes.DiscrepancyStatusPending,
	})
	require.NoError(t, err)
	require.Len(t, discrepanciesRes.Discrepancies, 1)

	escrowRes, err := client.AuditEscrow(ctx, &vtypes.QueryAuditEscrowRequest{Id: 7})
	require.NoError(t, err)
	require.Equal(t, provider.String(), escrowRes.Escrow.Provider)

	providerEscrowsRes, err := client.ProviderAuditEscrows(ctx, &vtypes.QueryProviderAuditEscrowsRequest{
		Provider:     provider.String(),
		StatusFilter: vtypes.AuditEscrowStatusOpen,
	})
	require.NoError(t, err)
	require.Len(t, providerEscrowsRes.Escrows, 1)

	graceRes, err := client.ProviderVerificationGrace(ctx, &vtypes.QueryProviderVerificationGraceRequest{
		Provider: provider.String(),
	})
	require.NoError(t, err)
	require.Equal(t, uint64(9), graceRes.Grace.ID)

	bondRes, err := client.ProviderBond(ctx, &vtypes.QueryProviderBondRequest{Provider: provider.String()})
	require.NoError(t, err)
	require.Equal(t, provider.String(), bondRes.Bond.Provider)
	require.Equal(t, int64(104500000), bondRes.RequiredForCurrentTier.Amount.Int64())

	snapshotRes, err := client.ProviderSnapshot(ctx, &vtypes.QueryProviderSnapshotRequest{Provider: provider.String()})
	require.NoError(t, err)
	require.Equal(t, []byte("snapshot"), snapshotRes.Snapshot.SnapshotHash)

	paramsRes, err := client.Params(ctx, &vtypes.QueryParamsRequest{})
	require.NoError(t, err)
	require.Equal(t, DefaultParams(), paramsRes.Params)
}

func TestGRPCQueryMissingRecords(t *testing.T) {
	ctx, k := setupStoreKeeper(t)
	client := queryClient(t, ctx, k)

	_, err := client.Auditor(ctx, &vtypes.QueryAuditorRequest{Auditor: testutil.AccAddress(t).String()})
	require.Error(t, err)

	_, err = client.ProviderSnapshot(ctx, &vtypes.QueryProviderSnapshotRequest{Provider: testutil.AccAddress(t).String()})
	require.Error(t, err)
}

func queryClient(t testing.TB, ctx sdk.Context, k Keeper) vtypes.QueryClient {
	t.Helper()

	cfg := testutilmod.MakeTestEncodingConfig()
	queryHelper := baseapp.NewQueryServerTestHelper(ctx, cfg.InterfaceRegistry)
	vtypes.RegisterQueryServer(queryHelper, k.NewQuerier())
	return vtypes.NewQueryClient(queryHelper)
}
