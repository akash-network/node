package keeper_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"

	"cosmossdk.io/log"
	"cosmossdk.io/store"
	storemetrics "cosmossdk.io/store/metrics"
	storetypes "cosmossdk.io/store/types"
	dbm "github.com/cosmos/cosmos-db"
	sdk "github.com/cosmos/cosmos-sdk/types"
	testutilmod "github.com/cosmos/cosmos-sdk/types/module/testutil"

	types "pkg.akt.dev/go/node/cert/v1"
	"pkg.akt.dev/go/testutil"

	"pkg.akt.dev/node/x/cert/keeper"
)

func TestCertKeeperCreate(t *testing.T) {
	ctx, keeper := setupKeeper(t)

	owner := testutil.AccAddress(t)

	cert := testutil.Certificate(t, owner)

	err := keeper.CreateCertificate(ctx, owner, cert.PEM.Cert, cert.PEM.Pub)
	require.NoError(t, err)

	resp, exists := keeper.GetCertificateByID(ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.True(t, exists)
	testutil.CertificateRequireEqualResponse(t, cert, resp, types.CertificateValid)
}

func TestCertKeeperCreateOwnerMismatch(t *testing.T) {
	ctx, keeper := setupKeeper(t)

	owner := testutil.AccAddress(t)

	cert := testutil.Certificate(t, owner)

	err := keeper.CreateCertificate(ctx, testutil.AccAddress(t), cert.PEM.Cert, cert.PEM.Pub)
	require.Error(t, err, types.ErrInvalidCertificateValue.Error())

	_, exists := keeper.GetCertificateByID(ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.False(t, exists)

	_, exists = keeper.GetCertificateByID(ctx, types.CertID{
		Owner: owner,
	})
	require.False(t, exists)
}

func TestCertKeeperMultipleActive(t *testing.T) {
	ctx, keeper := setupKeeper(t)

	owner := testutil.AccAddress(t)
	cert := testutil.Certificate(t, owner)

	err := keeper.CreateCertificate(ctx, owner, cert.PEM.Cert, cert.PEM.Pub)
	require.NoError(t, err)

	resp, exists := keeper.GetCertificateByID(ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.True(t, exists)
	testutil.CertificateRequireEqualResponse(t, cert, resp, types.CertificateValid)

	err = keeper.CreateCertificate(ctx, owner, cert.PEM.Cert, cert.PEM.Pub)
	require.Error(t, err, types.ErrCertificateExists.Error())

	cert1 := testutil.Certificate(t, owner)
	err = keeper.CreateCertificate(ctx, owner, cert1.PEM.Cert, cert1.PEM.Pub)
	require.NoError(t, err)

	resp, exists = keeper.GetCertificateByID(ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.True(t, exists)
	testutil.CertificateRequireEqualResponse(t, cert, resp, types.CertificateValid)

	resp, exists = keeper.GetCertificateByID(ctx, types.CertID{
		Owner:  owner,
		Serial: cert1.Serial,
	})
	require.True(t, exists)
	testutil.CertificateRequireEqualResponse(t, cert1, resp, types.CertificateValid)
}

func TestCertKeeperRevoke(t *testing.T) {
	ctx, keeper := setupKeeper(t)

	owner := testutil.AccAddress(t)

	cert := testutil.Certificate(t, owner)

	err := keeper.CreateCertificate(ctx, owner, cert.PEM.Cert, cert.PEM.Pub)
	require.NoError(t, err)

	resp, exists := keeper.GetCertificateByID(ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.True(t, exists)
	testutil.CertificateRequireEqualResponse(t, cert, resp, types.CertificateValid)

	err = keeper.RevokeCertificate(ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.NoError(t, err)

	resp, exists = keeper.GetCertificateByID(ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.True(t, exists)
	testutil.CertificateRequireEqualResponse(t, cert, resp, types.CertificateRevoked)

	err = keeper.RevokeCertificate(ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.Error(t, err, types.ErrCertificateAlreadyRevoked.Error())
}

func TestCertKeeperRevokeCreateRevoked(t *testing.T) {
	ctx, keeper := setupKeeper(t)

	owner := testutil.AccAddress(t)

	cert := testutil.Certificate(t, owner)

	err := keeper.CreateCertificate(ctx, owner, cert.PEM.Cert, cert.PEM.Pub)
	require.NoError(t, err)

	resp, exists := keeper.GetCertificateByID(ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.True(t, exists)
	testutil.CertificateRequireEqualResponse(t, cert, resp, types.CertificateValid)
	err = keeper.RevokeCertificate(ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.NoError(t, err)

	resp, exists = keeper.GetCertificateByID(ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.True(t, exists)
	testutil.CertificateRequireEqualResponse(t, cert, resp, types.CertificateRevoked)

	err = keeper.CreateCertificate(ctx, owner, cert.PEM.Cert, cert.PEM.Pub)
	require.Error(t, err, types.ErrCertificateExists.Error())
}

func TestCertKeeperRevokeCreate(t *testing.T) {
	ctx, keeper := setupKeeper(t)

	owner := testutil.AccAddress(t)

	cert := testutil.Certificate(t, owner)

	err := keeper.CreateCertificate(ctx, owner, cert.PEM.Cert, cert.PEM.Pub)
	require.NoError(t, err)

	resp, exists := keeper.GetCertificateByID(ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.True(t, exists)
	testutil.CertificateRequireEqualResponse(t, cert, resp, types.CertificateValid)
	err = keeper.RevokeCertificate(ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.NoError(t, err)

	resp, exists = keeper.GetCertificateByID(ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.True(t, exists)
	testutil.CertificateRequireEqualResponse(t, cert, resp, types.CertificateRevoked)

	cert1 := testutil.Certificate(t, owner)

	err = keeper.CreateCertificate(ctx, owner, cert1.PEM.Cert, cert1.PEM.Pub)
	require.NoError(t, err)
}

func setupKeeper(t testing.TB) (sdk.Context, keeper.Keeper) {
	t.Helper()

	cfg := testutilmod.MakeTestEncodingConfig()
	cdc := cfg.Codec

	key := storetypes.NewKVStoreKey(types.StoreKey)
	db := dbm.NewMemDB()
	ms := store.NewCommitMultiStore(db, log.NewNopLogger(), storemetrics.NewNoOpMetrics())
	ms.MountStoreWithDB(key, storetypes.StoreTypeIAVL, db)

	err := ms.LoadLatestVersion()
	require.NoError(t, err)

	ctx := sdk.NewContext(ms, tmproto.Header{Time: time.Unix(0, 0)}, false, testutil.Logger(t))
	return ctx, keeper.NewKeeper(cdc, key)
}
