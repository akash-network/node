package keeper_test

import (
	"reflect"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/provider/v1beta4"
	"pkg.akt.dev/go/testutil"

	"pkg.akt.dev/node/v2/testutil/state"
	"pkg.akt.dev/node/v2/x/provider/keeper"
)

func TestProviderCreate(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	ctx = ctx.WithBlockTime(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))
	prov := testutil.Provider(t)

	err := keeper.Create(ctx, prov)
	require.NoError(t, err)

	owner, err := sdk.AccAddressFromBech32(prov.Owner)
	require.NoError(t, err)

	foundProv, found := keeper.Get(ctx, owner)
	require.True(t, found)
	require.Equal(t, prov, foundProv)

	registration, found := keeper.GetRegistration(ctx, owner)
	require.True(t, found)
	require.Equal(t, prov.Owner, registration.Owner)
	require.Equal(t, ctx.BlockTime(), registration.RegisteredAt)
}

func TestProviderDuplicate(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	prov := testutil.Provider(t)

	err := keeper.Create(ctx, prov)
	require.NoError(t, err)

	err = keeper.Create(ctx, prov)
	require.EqualError(t, err, types.ErrProviderExists.Error())
}

func TestProviderGetNonExisting(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	prov := testutil.Provider(t)

	owner, err := sdk.AccAddressFromBech32(prov.Owner)
	require.NoError(t, err)

	foundProv, found := keeper.Get(ctx, owner)
	require.False(t, found)
	require.Equal(t, types.Provider{}, foundProv)
}

func TestProviderDeleteExisting(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	prov := testutil.Provider(t)

	err := keeper.Create(ctx, prov)
	require.NoError(t, err)

	owner, err := sdk.AccAddressFromBech32(prov.Owner)
	require.NoError(t, err)

	require.Panics(t, func() {
		keeper.Delete(ctx, owner)
	})

	foundProv, found := keeper.Get(ctx, owner)
	require.True(t, found)
	require.Equal(t, prov, foundProv)
}

func TestProviderUpdateNonExisting(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	prov := testutil.Provider(t)

	err := keeper.Update(ctx, prov)
	require.EqualError(t, err, types.ErrProviderNotFound.Error())
}

func TestProviderUpdateExisting(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	prov := testutil.Provider(t)

	err := keeper.Create(ctx, prov)
	require.NoError(t, err)

	prov.HostURI = "akash.domain.com"
	err = keeper.Update(ctx, prov)
	require.NoError(t, err)

	owner, err := sdk.AccAddressFromBech32(prov.Owner)
	require.NoError(t, err)

	foundProv, found := keeper.Get(ctx, owner)
	require.True(t, found)
	require.Equal(t, prov, foundProv)
}

func TestWithProviders(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	prov := testutil.Provider(t)
	prov2 := testutil.Provider(t)

	err := keeper.Create(ctx, prov)
	require.NoError(t, err)

	err = keeper.Create(ctx, prov2)
	require.NoError(t, err)

	count := 0

	keeper.WithProviders(ctx, func(provider types.Provider) bool {
		if !reflect.DeepEqual(provider, prov) && !reflect.DeepEqual(provider, prov2) {
			require.Fail(t, "unknown provider")
		}
		count++
		return false
	})

	require.Equal(t, 2, count)
}

func TestWithProvidersBreak(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	prov := testutil.Provider(t)
	prov2 := testutil.Provider(t)

	err := keeper.Create(ctx, prov)
	require.NoError(t, err)

	err = keeper.Create(ctx, prov2)
	require.NoError(t, err)

	count := 0

	keeper.WithProviders(ctx, func(provider types.Provider) bool {
		if !reflect.DeepEqual(provider, prov) && !reflect.DeepEqual(provider, prov2) {
			require.Fail(t, "unknown provider")
		}
		count++
		return true
	})

	require.Equal(t, 1, count)
}

func TestKeeperCoder(t *testing.T) {
	_, keeper := setupKeeper(t)
	codec := keeper.Codec()
	require.NotNil(t, codec)
}

func TestProviderParams(t *testing.T) {
	ctx, keeper := setupKeeper(t)

	params := keeper.GetParams(ctx)
	require.Equal(t, 7*24*time.Hour, params.MaintenanceMaxDuration)
	require.Equal(t, 90*24*time.Hour, params.MaintenanceMaxLookahead)

	params.MaintenanceMaxDuration = 24 * time.Hour
	err := keeper.SetParams(ctx, params)
	require.NoError(t, err)

	found := keeper.GetParams(ctx)
	require.Equal(t, params, found)
}

func TestProviderMaintenanceCRUD(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	prov := testutil.Provider(t)
	err := keeper.Create(ctx, prov)
	require.NoError(t, err)

	owner, err := sdk.AccAddressFromBech32(prov.Owner)
	require.NoError(t, err)

	id := keeper.AllocateMaintenanceID(ctx)
	require.Equal(t, uint64(1), id)
	require.Equal(t, uint64(2), keeper.GetNextMaintenanceID(ctx))

	record := types.ProviderMaintenanceRecord{
		ID:              id,
		Provider:        prov.Owner,
		MaintenanceType: types.ProviderMaintenanceType_provider_maintenance_type_planned,
		StartsAt:        ctx.BlockTime().Add(time.Hour),
		ExpectedEndsAt:  ctx.BlockTime().Add(2 * time.Hour),
		OpenedAt:        ctx.BlockTime(),
	}
	err = keeper.SetMaintenance(ctx, record)
	require.NoError(t, err)
	keeper.SetActiveMaintenanceID(ctx, owner, id)

	foundRecord, found := keeper.GetMaintenance(ctx, id)
	require.True(t, found)
	require.Equal(t, record, foundRecord)

	activeID, found := keeper.GetActiveMaintenanceID(ctx, owner)
	require.True(t, found)
	require.Equal(t, id, activeID)

	var records []types.ProviderMaintenanceRecord
	keeper.WithMaintenances(ctx, owner, func(record types.ProviderMaintenanceRecord) bool {
		records = append(records, record)
		return false
	})
	require.Equal(t, []types.ProviderMaintenanceRecord{record}, records)

	keeper.DeleteActiveMaintenanceID(ctx, owner)
	_, found = keeper.GetActiveMaintenanceID(ctx, owner)
	require.False(t, found)
}

func TestProviderMaintenanceReassignsOwnerIndex(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	prov := testutil.Provider(t)
	err := keeper.Create(ctx, prov)
	require.NoError(t, err)

	prov2 := testutil.Provider(t)
	err = keeper.Create(ctx, prov2)
	require.NoError(t, err)

	owner, err := sdk.AccAddressFromBech32(prov.Owner)
	require.NoError(t, err)
	owner2, err := sdk.AccAddressFromBech32(prov2.Owner)
	require.NoError(t, err)

	record := types.ProviderMaintenanceRecord{
		ID:              1,
		Provider:        prov.Owner,
		MaintenanceType: types.ProviderMaintenanceType_provider_maintenance_type_planned,
		StartsAt:        ctx.BlockTime().Add(time.Hour),
		ExpectedEndsAt:  ctx.BlockTime().Add(2 * time.Hour),
		OpenedAt:        ctx.BlockTime(),
	}
	err = keeper.SetMaintenance(ctx, record)
	require.NoError(t, err)

	record.Provider = prov2.Owner
	err = keeper.SetMaintenance(ctx, record)
	require.NoError(t, err)

	var records []types.ProviderMaintenanceRecord
	keeper.WithMaintenances(ctx, owner, func(record types.ProviderMaintenanceRecord) bool {
		records = append(records, record)
		return false
	})
	require.Empty(t, records)

	keeper.WithMaintenances(ctx, owner2, func(record types.ProviderMaintenanceRecord) bool {
		records = append(records, record)
		return false
	})
	require.Equal(t, []types.ProviderMaintenanceRecord{record}, records)
}

func setupKeeper(t testing.TB) (sdk.Context, keeper.IKeeper) {
	t.Helper()

	suite := state.SetupTestSuite(t)

	return suite.Context(), suite.ProviderKeeper()
}
