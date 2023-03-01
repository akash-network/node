package keeper_test

import (
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/store"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/rand"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	dbm "github.com/tendermint/tm-db"

	types "github.com/akash-network/akash-api/go/node/audit/v1beta3"

	"github.com/akash-network/node/testutil"
	"github.com/akash-network/node/x/audit/keeper"
)

func TestProviderCreate(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	id, prov := testutil.AuditedProvider(t)

	err := keeper.CreateOrUpdateProviderAttributes(ctx, id, prov.Attributes)
	require.NoError(t, err)

	foundProv, found := keeper.GetProviderAttributes(ctx, id.Owner)
	require.True(t, found)
	require.Equal(t, types.Providers{prov}, foundProv)
}

func TestProviderUpdateAppendNewAttributes(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	id, prov := testutil.AuditedProvider(t)

	err := keeper.CreateOrUpdateProviderAttributes(ctx, id, prov.Attributes)
	require.NoError(t, err)

	attr := prov.Attributes
	prov.Attributes = testutil.Attributes(t)

	attr = append(attr, prov.Attributes...)

	sort.SliceStable(attr, func(i, j int) bool {
		return attr[i].Key < attr[j].Key
	})

	err = keeper.CreateOrUpdateProviderAttributes(ctx, id, prov.Attributes)
	require.NoError(t, err)

	prov.Attributes = attr

	foundProv, found := keeper.GetProviderAttributes(ctx, id.Owner)
	require.True(t, found)
	require.Equal(t, types.Providers{prov}, foundProv)
}

func TestProviderUpdateOverrideAttributes(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	id, prov := testutil.AuditedProvider(t)

	err := keeper.CreateOrUpdateProviderAttributes(ctx, id, prov.Attributes)
	require.NoError(t, err)

	for i := range prov.Attributes {
		prov.Attributes[i].Value = strconv.FormatInt(rand.Int64(), 10)
	}

	sort.SliceStable(prov.Attributes, func(i, j int) bool {
		return prov.Attributes[i].Key < prov.Attributes[j].Key
	})

	err = keeper.CreateOrUpdateProviderAttributes(ctx, id, prov.Attributes)
	require.NoError(t, err)

	foundProv, found := keeper.GetProviderAttributes(ctx, id.Owner)
	require.True(t, found)
	require.Equal(t, types.Providers{prov}, foundProv)
}

func TestProviderDeleteExistingAttributes(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	id, prov := testutil.AuditedProvider(t)

	// lets append some more attributes in case testutil generated only 1
	prov.Attributes = append(prov.Attributes, testutil.Attributes(t)...)

	err := keeper.CreateOrUpdateProviderAttributes(ctx, id, prov.Attributes)
	require.NoError(t, err)

	err = keeper.DeleteProviderAttributes(ctx, id, []string{prov.Attributes[0].Key})
	require.NoError(t, err)

	prov.Attributes = prov.Attributes[1:]

	sort.SliceStable(prov.Attributes, func(i, j int) bool {
		return prov.Attributes[i].Key < prov.Attributes[j].Key
	})

	foundProv, found := keeper.GetProviderAttributes(ctx, id.Owner)
	require.True(t, found)
	require.Equal(t, types.Providers{prov}, foundProv)
}

func TestProviderDeleteNonExistingAttributes(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	id, prov := testutil.AuditedProvider(t)

	err := keeper.CreateOrUpdateProviderAttributes(ctx, id, prov.Attributes)
	require.NoError(t, err)

	attr := testutil.Attributes(t)
	keys := make([]string, len(attr))

	for _, entry := range attr {
		keys = append(keys, entry.Key)
	}

	err = keeper.DeleteProviderAttributes(ctx, id, keys)
	require.Error(t, err)
}

func TestProviderDeleteExisting(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	id, prov := testutil.AuditedProvider(t)

	err := keeper.CreateOrUpdateProviderAttributes(ctx, id, prov.Attributes)
	require.NoError(t, err)

	err = keeper.DeleteProviderAttributes(ctx, id, nil)
	require.NoError(t, err)

	err = keeper.DeleteProviderAttributes(ctx, id, nil)
	require.EqualError(t, err, types.ErrProviderNotFound.Error())
}

func TestProviderDeleteNonExisting(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	id, _ := testutil.AuditedProvider(t)

	err := keeper.DeleteProviderAttributes(ctx, id, nil)
	require.EqualError(t, err, types.ErrProviderNotFound.Error())
}

func TestKeeperCoder(t *testing.T) {
	_, keeper := setupKeeper(t)
	codec := keeper.Codec()
	require.NotNil(t, codec)
}

func setupKeeper(t testing.TB) (sdk.Context, keeper.Keeper) {
	t.Helper()
	key := sdk.NewKVStoreKey(types.StoreKey)
	db := dbm.NewMemDB()
	ms := store.NewCommitMultiStore(db)
	ms.MountStoreWithDB(key, sdk.StoreTypeIAVL, db)
	err := ms.LoadLatestVersion()
	require.NoError(t, err)
	ctx := sdk.NewContext(ms, tmproto.Header{Time: time.Unix(0, 0)}, false, testutil.Logger(t))
	return ctx, keeper.NewKeeper(types.ModuleCdc, key)
}
