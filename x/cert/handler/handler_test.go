package handler_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"

	"cosmossdk.io/log"
	"cosmossdk.io/store"
	storemetrics "cosmossdk.io/store/metrics"
	storetypes "cosmossdk.io/store/types"
	dbm "github.com/cosmos/cosmos-db"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdktestdata "github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	testutilmod "github.com/cosmos/cosmos-sdk/types/module/testutil"

	types "pkg.akt.dev/go/node/cert/v1"

	"pkg.akt.dev/node/testutil"
	"pkg.akt.dev/node/x/cert/handler"
	"pkg.akt.dev/node/x/cert/keeper"
)

type testSuite struct {
	t       testing.TB
	ms      storetypes.CommitMultiStore
	ctx     sdk.Context
	keeper  keeper.Keeper
	handler baseapp.MsgServiceHandler
}

func setupTestSuite(t *testing.T) *testSuite {
	suite := &testSuite{
		t: t,
	}

	cfg := testutilmod.MakeTestEncodingConfig()
	cdc := cfg.Codec

	aKey := storetypes.NewTransientStoreKey(types.StoreKey)

	db := dbm.NewMemDB()
	suite.ms = store.NewCommitMultiStore(db, log.NewNopLogger(), storemetrics.NewNoOpMetrics())
	suite.ms.MountStoreWithDB(aKey, storetypes.StoreTypeIAVL, db)

	err := suite.ms.LoadLatestVersion()
	require.NoError(t, err)

	suite.ctx = sdk.NewContext(suite.ms, tmproto.Header{}, true, testutil.Logger(t))

	suite.keeper = keeper.NewKeeper(cdc, aKey)

	suite.handler = handler.NewHandler(suite.keeper)

	return suite
}

func TestCertHandlerBadMessageType(t *testing.T) {
	suite := setupTestSuite(t)

	_, err := suite.handler(suite.ctx, sdk.Msg(sdktestdata.NewTestMsg()))
	require.Error(t, err)
	require.True(t, errors.Is(err, sdkerrors.ErrUnknownRequest))
}

func TestCertHandlerCreate(t *testing.T) {
	suite := setupTestSuite(t)

	owner := testutil.AccAddress(t)

	cert := testutil.Certificate(t, owner)

	msg := &types.MsgCreateCertificate{
		Owner:  owner.String(),
		Cert:   cert.PEM.Cert,
		Pubkey: cert.PEM.Pub,
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NotNil(t, res)
	require.NoError(t, err)

	resp, exists := suite.keeper.GetCertificateByID(suite.ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.True(t, exists)
	testutil.CertificateRequireEqualResponse(t, cert, resp, types.CertificateValid)
}

func TestCertHandlerCreateOwnerMismatch(t *testing.T) {
	suite := setupTestSuite(t)

	owner := testutil.AccAddress(t)

	cert := testutil.Certificate(t, owner)

	msg := &types.MsgCreateCertificate{
		Owner:  testutil.AccAddress(t).String(),
		Cert:   cert.PEM.Cert,
		Pubkey: cert.PEM.Pub,
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.Error(t, err, types.ErrInvalidCertificateValue.Error())

	_, exists := suite.keeper.GetCertificateByID(suite.ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.False(t, exists)

	_, exists = suite.keeper.GetCertificateByID(suite.ctx, types.CertID{
		Owner: owner,
	})
	require.False(t, exists)
}

func TestCertHandlerDuplicate(t *testing.T) {
	suite := setupTestSuite(t)

	owner := testutil.AccAddress(t)

	cert := testutil.Certificate(t, owner)

	msg := &types.MsgCreateCertificate{
		Owner:  owner.String(),
		Cert:   cert.PEM.Cert,
		Pubkey: cert.PEM.Pub,
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NotNil(t, res)
	require.NoError(t, err)

	resp, exists := suite.keeper.GetCertificateByID(suite.ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.True(t, exists)
	testutil.CertificateRequireEqualResponse(t, cert, resp, types.CertificateValid)

	res, err = suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.Error(t, err, types.ErrCertificateExists.Error())

	cert1 := testutil.Certificate(t, owner)
	msg = &types.MsgCreateCertificate{
		Owner:  owner.String(),
		Cert:   cert1.PEM.Cert,
		Pubkey: cert1.PEM.Pub,
	}

	res, err = suite.handler(suite.ctx, msg)
	require.NotNil(t, res)
	require.NoError(t, err)

	resp, exists = suite.keeper.GetCertificateByID(suite.ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.True(t, exists)
	testutil.CertificateRequireEqualResponse(t, cert, resp, types.CertificateValid)

	resp, exists = suite.keeper.GetCertificateByID(suite.ctx, types.CertID{
		Owner:  owner,
		Serial: cert1.Serial,
	})
	require.True(t, exists)
	testutil.CertificateRequireEqualResponse(t, cert1, resp, types.CertificateValid)
}

func TestCertHandlerRevoke(t *testing.T) {
	suite := setupTestSuite(t)

	owner := testutil.AccAddress(t)

	cert := testutil.Certificate(t, owner)

	msgCreate := &types.MsgCreateCertificate{
		Owner:  owner.String(),
		Cert:   cert.PEM.Cert,
		Pubkey: cert.PEM.Pub,
	}

	res, err := suite.handler(suite.ctx, msgCreate)
	require.NotNil(t, res)
	require.NoError(t, err)

	resp, exists := suite.keeper.GetCertificateByID(suite.ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.True(t, exists)
	testutil.CertificateRequireEqualResponse(t, cert, resp, types.CertificateValid)

	msgRevoke := &types.MsgRevokeCertificate{
		ID: types.ID{
			Owner:  owner.String(),
			Serial: cert.Serial.String(),
		},
	}

	res, err = suite.handler(suite.ctx, msgRevoke)
	require.NotNil(t, res)
	require.NoError(t, err)

	resp, exists = suite.keeper.GetCertificateByID(suite.ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.True(t, exists)
	testutil.CertificateRequireEqualResponse(t, cert, resp, types.CertificateRevoked)

	res, err = suite.handler(suite.ctx, msgRevoke)
	require.Nil(t, res)
	require.Error(t, err, types.ErrCertificateAlreadyRevoked.Error())
}

func TestCertHandlerRevokeCreateRevoked(t *testing.T) {
	suite := setupTestSuite(t)

	owner := testutil.AccAddress(t)

	cert := testutil.Certificate(t, owner)

	msgCreate := &types.MsgCreateCertificate{
		Owner:  owner.String(),
		Cert:   cert.PEM.Cert,
		Pubkey: cert.PEM.Pub,
	}

	res, err := suite.handler(suite.ctx, msgCreate)
	require.NotNil(t, res)
	require.NoError(t, err)

	resp, exists := suite.keeper.GetCertificateByID(suite.ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.True(t, exists)
	testutil.CertificateRequireEqualResponse(t, cert, resp, types.CertificateValid)

	msgRevoke := &types.MsgRevokeCertificate{
		ID: types.ID{
			Owner:  owner.String(),
			Serial: cert.Serial.String(),
		},
	}

	res, err = suite.handler(suite.ctx, msgRevoke)
	require.NotNil(t, res)
	require.NoError(t, err)

	resp, exists = suite.keeper.GetCertificateByID(suite.ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.True(t, exists)
	testutil.CertificateRequireEqualResponse(t, cert, resp, types.CertificateRevoked)

	res, err = suite.handler(suite.ctx, msgCreate)
	require.Nil(t, res)
	require.Error(t, err, types.ErrCertificateExists.Error())
}

func TestCertHandlerRevokeCreate(t *testing.T) {
	suite := setupTestSuite(t)
	owner := testutil.AccAddress(t)
	cert := testutil.Certificate(t, owner)

	msgCreate := &types.MsgCreateCertificate{
		Owner:  owner.String(),
		Cert:   cert.PEM.Cert,
		Pubkey: cert.PEM.Pub,
	}

	res, err := suite.handler(suite.ctx, msgCreate)
	require.NotNil(t, res)
	require.NoError(t, err)

	resp, exists := suite.keeper.GetCertificateByID(suite.ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.True(t, exists)
	testutil.CertificateRequireEqualResponse(t, cert, resp, types.CertificateValid)

	msgRevoke := &types.MsgRevokeCertificate{
		ID: types.ID{
			Owner:  owner.String(),
			Serial: cert.Serial.String(),
		},
	}

	res, err = suite.handler(suite.ctx, msgRevoke)
	require.NotNil(t, res)
	require.NoError(t, err)

	resp, exists = suite.keeper.GetCertificateByID(suite.ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.True(t, exists)
	testutil.CertificateRequireEqualResponse(t, cert, resp, types.CertificateRevoked)

	cert1 := testutil.Certificate(t, owner)

	msgCreate = &types.MsgCreateCertificate{
		Owner:  owner.String(),
		Cert:   cert1.PEM.Cert,
		Pubkey: cert1.PEM.Pub,
	}

	res, err = suite.handler(suite.ctx, msgCreate)
	require.NotNil(t, res)
	require.NoError(t, err)
}
