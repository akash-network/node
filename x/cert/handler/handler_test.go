package handler_test

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/store"
	sdktestdata "github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	dbm "github.com/tendermint/tm-db"

	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/cert/handler"
	"github.com/ovrclk/akash/x/cert/keeper"
	"github.com/ovrclk/akash/x/cert/types"
)

type testSuite struct {
	t       testing.TB
	ms      sdk.CommitMultiStore
	ctx     sdk.Context
	keeper  keeper.Keeper
	handler sdk.Handler
}

func setupTestSuite(t *testing.T) *testSuite {
	suite := &testSuite{
		t: t,
	}

	aKey := sdk.NewTransientStoreKey(types.StoreKey)

	db := dbm.NewMemDB()
	suite.ms = store.NewCommitMultiStore(db)
	suite.ms.MountStoreWithDB(aKey, sdk.StoreTypeIAVL, db)

	err := suite.ms.LoadLatestVersion()
	require.NoError(t, err)

	suite.ctx = sdk.NewContext(suite.ms, tmproto.Header{}, true, testutil.Logger(t))

	suite.keeper = keeper.NewKeeper(types.ModuleCdc, aKey)

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
	require.Equal(t, types.CertificateValid, resp.State)
	require.Equal(t, cert.PEM.Cert, resp.Cert)
	require.Equal(t, cert.PEM.Pub, resp.Pubkey)
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
	require.Equal(t, types.CertificateValid, resp.State)
	require.Equal(t, cert.PEM.Cert, resp.Cert)
	require.Equal(t, cert.PEM.Pub, resp.Pubkey)

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
	require.Nil(t, res)
	require.Error(t, err, types.ErrCertificateForAccountAlreadyExists.Error())

	resp, exists = suite.keeper.GetCertificateByID(suite.ctx, types.CertID{
		Owner:  owner,
		Serial: cert.Serial,
	})
	require.True(t, exists)
	require.Equal(t, types.CertificateValid, resp.State)
	require.Equal(t, cert.PEM.Cert, resp.Cert)
	require.Equal(t, cert.PEM.Pub, resp.Pubkey)
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
	require.Equal(t, types.CertificateValid, resp.State)
	require.Equal(t, cert.PEM.Cert, resp.Cert)
	require.Equal(t, cert.PEM.Pub, resp.Pubkey)

	msgRevoke := &types.MsgRevokeCertificate{
		ID: types.CertificateID{
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
	require.Equal(t, types.CertificateRevoked, resp.State)
	require.Equal(t, cert.PEM.Cert, resp.Cert)
	require.Equal(t, cert.PEM.Pub, resp.Pubkey)

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
	require.Equal(t, types.CertificateValid, resp.State)
	require.Equal(t, cert.PEM.Cert, resp.Cert)
	require.Equal(t, cert.PEM.Pub, resp.Pubkey)

	msgRevoke := &types.MsgRevokeCertificate{
		ID: types.CertificateID{
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
	require.Equal(t, types.CertificateRevoked, resp.State)
	require.Equal(t, cert.PEM.Cert, resp.Cert)
	require.Equal(t, cert.PEM.Pub, resp.Pubkey)

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
	require.Equal(t, types.CertificateValid, resp.State)
	require.Equal(t, cert.PEM.Cert, resp.Cert)
	require.Equal(t, cert.PEM.Pub, resp.Pubkey)

	msgRevoke := &types.MsgRevokeCertificate{
		ID: types.CertificateID{
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
	require.Equal(t, types.CertificateRevoked, resp.State)
	require.Equal(t, cert.PEM.Cert, resp.Cert)
	require.Equal(t, cert.PEM.Pub, resp.Pubkey)

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
