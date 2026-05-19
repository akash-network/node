package provider_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/provider/v1beta4"
	"pkg.akt.dev/go/testutil"

	"pkg.akt.dev/node/v2/testutil/state"
	provider "pkg.akt.dev/node/v2/x/provider"
	"pkg.akt.dev/node/v2/x/provider/keeper"
)

func TestDefaultGenesisState(t *testing.T) {
	genesis := provider.DefaultGenesisState()

	require.Equal(t, keeper.DefaultParams(), genesis.Params)
	require.Equal(t, uint64(1), genesis.NextMaintenanceID)
}

func TestProviderGenesisImportExport(t *testing.T) {
	suite := state.SetupTestSuite(t)
	ctx := suite.Context().WithBlockTime(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))
	prov := testutil.Provider(t)
	prov.HostURI = testutil.ProviderHostname(t)

	genesis := &types.GenesisState{
		Providers: types.Providers{prov},
		Params:    keeper.DefaultParams(),
		Maintenances: []types.ProviderMaintenanceRecord{
			{
				ID:              1,
				Provider:        prov.Owner,
				MaintenanceType: types.ProviderMaintenanceType_provider_maintenance_type_planned,
				StartsAt:        ctx.BlockTime().Add(time.Hour),
				ExpectedEndsAt:  ctx.BlockTime().Add(2 * time.Hour),
				OpenedAt:        ctx.BlockTime(),
			},
		},
		NextMaintenanceID: 2,
		Registrations: []types.ProviderRegistration{
			{
				Owner:        prov.Owner,
				RegisteredAt: ctx.BlockTime(),
			},
		},
	}

	err := provider.ValidateGenesis(genesis)
	require.NoError(t, err)

	provider.InitGenesis(ctx, suite.ProviderKeeper(), genesis)
	exported := provider.ExportGenesis(ctx, suite.ProviderKeeper())

	require.Equal(t, genesis.Providers, exported.Providers)
	require.Equal(t, genesis.Params, exported.Params)
	require.Equal(t, genesis.Maintenances, exported.Maintenances)
	require.Equal(t, genesis.NextMaintenanceID, exported.NextMaintenanceID)
	require.Equal(t, genesis.Registrations, exported.Registrations)
}

func TestProviderGenesisBackfillsMissingRegistration(t *testing.T) {
	suite := state.SetupTestSuite(t)
	ctx := suite.Context().WithBlockTime(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))
	prov := testutil.Provider(t)
	prov.HostURI = testutil.ProviderHostname(t)

	genesis := &types.GenesisState{
		Providers:         types.Providers{prov},
		Params:            keeper.DefaultParams(),
		NextMaintenanceID: 1,
	}

	err := provider.ValidateGenesis(genesis)
	require.NoError(t, err)

	provider.InitGenesis(ctx, suite.ProviderKeeper(), genesis)
	exported := provider.ExportGenesis(ctx, suite.ProviderKeeper())

	require.Len(t, exported.Registrations, 1)
	require.Equal(t, prov.Owner, exported.Registrations[0].Owner)
	require.Equal(t, ctx.BlockTime(), exported.Registrations[0].RegisteredAt)
}
