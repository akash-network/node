package verification

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cosmossdk.io/log"
	"cosmossdk.io/store"
	storemetrics "cosmossdk.io/store/metrics"
	storetypes "cosmossdk.io/store/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	sdk "github.com/cosmos/cosmos-sdk/types"
	testutilmod "github.com/cosmos/cosmos-sdk/types/module/testutil"

	vtypes "pkg.akt.dev/go/node/verification/v1"
	"pkg.akt.dev/go/testutil"

	"pkg.akt.dev/node/v2/x/verification/keeper"
	moduletypes "pkg.akt.dev/node/v2/x/verification/types"
)

func TestGenesisRoundTrip(t *testing.T) {
	ctx, k := setupGenesisKeeper(t)
	provider := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	auditorB := testutil.AccAddress(t)

	state := &vtypes.GenesisState{
		Params:             keeper.DefaultParams(),
		Auditors:           []vtypes.AuditorRecord{genesisAuditor(auditor)},
		Attestations:       []vtypes.AttestationRecord{genesisAttestation(provider, auditor)},
		Discrepancies:      []vtypes.DiscrepancyEvent{genesisDiscrepancy(provider, auditor, auditorB)},
		ProviderBonds:      []vtypes.ProviderBondRecord{genesisProviderBond(provider)},
		ProviderSnapshots:  []vtypes.ProviderSnapshotRecord{genesisProviderSnapshot(provider)},
		NextDiscrepancyID:  12,
		AuditEscrows:       []vtypes.AuditEscrowRecord{genesisAuditEscrow(provider, auditor)},
		NextAuditEscrowID:  8,
		VerificationGraces: []vtypes.ProviderVerificationGraceRecord{genesisGrace(provider)},
		NextGraceRecordID:  10,
	}

	InitGenesis(ctx, k, state)
	exported := ExportGenesis(ctx, k)

	require.Equal(t, state.Params, exported.Params)
	require.Equal(t, state.NextDiscrepancyID, exported.NextDiscrepancyID)
	require.Equal(t, state.NextAuditEscrowID, exported.NextAuditEscrowID)
	require.Equal(t, state.NextGraceRecordID, exported.NextGraceRecordID)
	require.ElementsMatch(t, state.Auditors, exported.Auditors)
	require.ElementsMatch(t, state.Attestations, exported.Attestations)
	require.ElementsMatch(t, state.Discrepancies, exported.Discrepancies)
	require.ElementsMatch(t, state.ProviderBonds, exported.ProviderBonds)
	require.ElementsMatch(t, state.ProviderSnapshots, exported.ProviderSnapshots)
	require.ElementsMatch(t, state.AuditEscrows, exported.AuditEscrows)
	require.ElementsMatch(t, state.VerificationGraces, exported.VerificationGraces)
}

func TestDefaultGenesisState(t *testing.T) {
	state := DefaultGenesisState()

	require.Equal(t, keeper.DefaultParams(), state.Params)
	require.Equal(t, uint64(1), state.NextDiscrepancyID)
	require.Equal(t, uint64(1), state.NextAuditEscrowID)
	require.Equal(t, uint64(1), state.NextGraceRecordID)
}

func setupGenesisKeeper(t testing.TB) (sdk.Context, keeper.Keeper) {
	t.Helper()

	cfg := testutilmod.MakeTestEncodingConfig()
	key := storetypes.NewKVStoreKey(moduletypes.StoreKey)
	db := dbm.NewMemDB()

	ms := store.NewCommitMultiStore(db, log.NewNopLogger(), storemetrics.NewNoOpMetrics())
	ms.MountStoreWithDB(key, storetypes.StoreTypeIAVL, db)

	err := ms.LoadLatestVersion()
	require.NoError(t, err)

	ctx := sdk.NewContext(ms, tmproto.Header{Time: genesisNow()}, false, testutil.Logger(t))
	return ctx, keeper.NewKeeper(cfg.Codec, key)
}

func genesisAuditor(auditor sdk.AccAddress) vtypes.AuditorRecord {
	return vtypes.AuditorRecord{
		Address:            auditor.String(),
		Status:             vtypes.AuditorStatusActive,
		MaxAttestationTier: vtypes.TierTrusted,
		BondAmount:         sdk.NewInt64Coin("uakt", 100000000000),
		BondStatus:         vtypes.BondStatusBonded,
		RegisteredAt:       genesisNow(),
		RenewalDeadline:    genesisNow().Add(365 * 24 * time.Hour),
	}
}

func genesisAttestation(provider, auditor sdk.AccAddress) vtypes.AttestationRecord {
	return vtypes.AttestationRecord{
		Provider:      provider.String(),
		Auditor:       auditor.String(),
		Tier:          vtypes.TierVerified,
		EvidenceHash:  []byte("evidence"),
		Fee:           sdk.NewInt64Coin("uakt", 10000000),
		FeeStatus:     vtypes.FeeStatusEscrowed,
		CreatedAt:     genesisNow(),
		ExpiresAt:     genesisNow().Add(90 * 24 * time.Hour),
		Status:        vtypes.AttestationStatusValid,
		Deposit:       sdk.NewInt64Coin("uakt", 100000000),
		DepositStatus: vtypes.DepositStatusEscrowed,
		AuditEscrowID: 7,
	}
}

func genesisProviderBond(provider sdk.AccAddress) vtypes.ProviderBondRecord {
	return vtypes.ProviderBondRecord{
		Provider:     provider.String(),
		BondedAmount: sdk.NewInt64Coin("uakt", 200000000),
	}
}

func genesisProviderSnapshot(provider sdk.AccAddress) vtypes.ProviderSnapshotRecord {
	return vtypes.ProviderSnapshotRecord{
		Provider:           provider.String(),
		SnapshotHash:       []byte("snapshot"),
		ResourceSummary:    vtypes.ResourceSummary{TotalGPUs: 1, TotalVCPUs: 2},
		PostedAt:           genesisNow(),
		SnapshotTimestamp:  genesisNow(),
		ComplianceDeadline: genesisNow().Add(24 * time.Hour),
	}
}

func genesisAuditEscrow(provider, auditor sdk.AccAddress) vtypes.AuditEscrowRecord {
	return vtypes.AuditEscrowRecord{
		ID:                    7,
		Provider:              provider.String(),
		ConsumedByAuditor:     auditor.String(),
		RequestedTier:         vtypes.TierVerified,
		Fee:                   sdk.NewInt64Coin("uakt", 10000000),
		FeeStatus:             vtypes.FeeStatusEscrowed,
		ProviderDeposit:       sdk.NewInt64Coin("uakt", 100000000),
		ProviderDepositStatus: vtypes.ProviderDepositStatusEscrowed,
		Status:                vtypes.AuditEscrowStatusOpen,
		OpenedAt:              genesisNow(),
		ExpiresAt:             genesisNow().Add(24 * time.Hour),
	}
}

func genesisGrace(provider sdk.AccAddress) vtypes.ProviderVerificationGraceRecord {
	return vtypes.ProviderVerificationGraceRecord{
		ID:                   9,
		Provider:             provider.String(),
		PreservedTier:        vtypes.TierVerified,
		SourceDiscrepancyIDs: []uint64{11},
		StartedAt:            genesisNow(),
		ExpiresAt:            genesisNow().Add(14 * 24 * time.Hour),
		Status:               vtypes.VerificationGraceStatusActive,
	}
}

func genesisDiscrepancy(provider, auditorA, auditorB sdk.AccAddress) vtypes.DiscrepancyEvent {
	return vtypes.DiscrepancyEvent{
		ID:               11,
		Provider:         provider.String(),
		AuditorA:         auditorA.String(),
		AuditorATier:     vtypes.TierVerified,
		AuditorB:         auditorB.String(),
		AuditorBTier:     vtypes.TierEstablished,
		Timestamp:        genesisNow(),
		ResolutionStatus: vtypes.DiscrepancyStatusPending,
	}
}

func genesisNow() time.Time {
	return time.Unix(1700000000, 0).UTC()
}
