package keeper_test

import (
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/store"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	dbm "github.com/tendermint/tm-db"

	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/cert/keeper"
	"github.com/ovrclk/akash/x/cert/types"
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
	require.Equal(t, types.CertificateValid, resp.State)
	require.Equal(t, cert.PEM.Cert, resp.Cert)
	require.Equal(t, cert.PEM.Pub, resp.Pubkey)
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
	require.Equal(t, types.CertificateValid, resp.State)
	require.Equal(t, cert.PEM.Cert, resp.Cert)
	require.Equal(t, cert.PEM.Pub, resp.Pubkey)

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
	require.Equal(t, types.CertificateValid, resp.State)
	require.Equal(t, cert.PEM.Cert, resp.Cert)
	require.Equal(t, cert.PEM.Pub, resp.Pubkey)

	resp, exists = keeper.GetCertificateByID(ctx, types.CertID{
		Owner:  owner,
		Serial: cert1.Serial,
	})
	require.True(t, exists)
	require.Equal(t, types.CertificateValid, resp.State)
	require.Equal(t, cert1.PEM.Cert, resp.Cert)
	require.Equal(t, cert1.PEM.Pub, resp.Pubkey)
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
	require.Equal(t, types.CertificateValid, resp.State)
	require.Equal(t, cert.PEM.Cert, resp.Cert)
	require.Equal(t, cert.PEM.Pub, resp.Pubkey)

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
	require.Equal(t, types.CertificateRevoked, resp.State)
	require.Equal(t, cert.PEM.Cert, resp.Cert)
	require.Equal(t, cert.PEM.Pub, resp.Pubkey)

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
	require.Equal(t, types.CertificateValid, resp.State)
	require.Equal(t, cert.PEM.Cert, resp.Cert)
	require.Equal(t, cert.PEM.Pub, resp.Pubkey)

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
	require.Equal(t, types.CertificateRevoked, resp.State)
	require.Equal(t, cert.PEM.Cert, resp.Cert)
	require.Equal(t, cert.PEM.Pub, resp.Pubkey)

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
	require.Equal(t, types.CertificateValid, resp.State)
	require.Equal(t, cert.PEM.Cert, resp.Cert)
	require.Equal(t, cert.PEM.Pub, resp.Pubkey)

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
	require.Equal(t, types.CertificateRevoked, resp.State)
	require.Equal(t, cert.PEM.Cert, resp.Cert)
	require.Equal(t, cert.PEM.Pub, resp.Pubkey)

	cert1 := testutil.Certificate(t, owner)
	err = keeper.CreateCertificate(ctx, owner, cert1.PEM.Cert, cert1.PEM.Pub)
	require.NoError(t, err)
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
