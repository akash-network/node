package app_test

import (
	"testing"

	app_ "github.com/ovrclk/akash/app"
	dapp_ "github.com/ovrclk/akash/app/deployment"
	fapp_ "github.com/ovrclk/akash/app/fulfillment"
	lapp_ "github.com/ovrclk/akash/app/lease"
	oapp_ "github.com/ovrclk/akash/app/order"
	papp_ "github.com/ovrclk/akash/app/provider"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/code"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	abci_types "github.com/tendermint/tendermint/abci/types"
)

func TestApp(t *testing.T) {

	const balance = 1

	nonce := uint64(1)

	signer, keyfrom := testutil.PrivateKeySigner(t)
	addrfrom := keyfrom.PubKey().Address().Bytes()

	keyto := testutil.PrivateKey(t)
	addrto := keyto.PubKey().Address().Bytes()

	commitState, cacheState := testutil.NewState(t, &types.Genesis{
		Accounts: []types.Account{
			{Address: addrfrom, Balance: balance, Nonce: nonce},
		},
	})

	app, err := app_.Create(commitState, cacheState, testutil.Logger())
	require.NoError(t, err)

	{
		nonce := uint64(0)
		tx, err := txutil.BuildTx(signer, nonce, &types.TxSend{
			From:   addrfrom,
			To:     addrto,
			Amount: 0,
		})
		require.NoError(t, err)
		resp := app.DeliverTx(abci_types.RequestDeliverTx{Tx: tx})
		require.Equal(t, code.INVALID_TRANSACTION, resp.Code)
		require.True(t, resp.IsErr())
		require.False(t, resp.IsOK())
	}

	{
		nonce := uint64(1)
		tx, err := txutil.BuildTx(signer, nonce, &types.TxSend{
			From:   addrfrom,
			To:     addrto,
			Amount: 0,
		})
		require.NoError(t, err)
		resp := app.DeliverTx(abci_types.RequestDeliverTx{Tx: tx})
		require.Equal(t, code.INVALID_TRANSACTION, resp.Code)
		require.True(t, resp.IsErr())
		require.False(t, resp.IsOK())
	}

	{
		nonce := uint64(2)
		tx, err := txutil.BuildTx(signer, nonce, &types.TxSend{
			From:   addrfrom,
			To:     addrto,
			Amount: 0,
		})
		require.NoError(t, err)
		resp := app.DeliverTx(abci_types.RequestDeliverTx{Tx: tx})
		require.Equal(t, code.OK, resp.Code)
		require.False(t, resp.IsErr())
		require.True(t, resp.IsOK())
	}

	{
		nonce := uint64(3)
		tx, err := txutil.BuildTx(signer, nonce, &types.TxSend{
			From:   addrfrom,
			To:     addrto,
			Amount: 0,
		})
		require.NoError(t, err)
		resp := app.CheckTx(abci_types.RequestCheckTx{Tx: tx})
		require.Equal(t, code.OK, resp.Code)
		require.False(t, resp.IsErr())
		require.True(t, resp.IsOK())
	}

}

func TestLeaseTransfer(t *testing.T) {
	const (
		price   = 10
		balance = 100000
	)
	nonce := uint64(1)

	_, keyfrom := testutil.PrivateKeySigner(t)
	addrfrom := keyfrom.PubKey().Address().Bytes()

	keyto := testutil.PrivateKey(t)
	addrto := keyto.PubKey().Address().Bytes()

	commitState, cacheState := testutil.NewState(t, &types.Genesis{
		Accounts: []types.Account{
			{Address: addrfrom, Balance: balance, Nonce: nonce},
			{Address: addrto, Balance: 0, Nonce: nonce},
		},
	})

	tacct, err := cacheState.Account().Get(addrfrom)
	require.NoError(t, err)

	pacct, err := cacheState.Account().Get(addrto)
	require.NoError(t, err)

	nonce++

	app, err := app_.Create(commitState, cacheState, testutil.Logger())
	require.NoError(t, err)

	dapp := app.App(dapp_.Name)
	require.NotNil(t, dapp)

	oapp := app.App(oapp_.Name)
	require.NotNil(t, oapp)

	lapp := app.App(lapp_.Name)
	require.NotNil(t, lapp)

	papp := app.App(papp_.Name)
	require.NotNil(t, papp)

	fapp := app.App(fapp_.Name)
	require.NotNil(t, fapp)

	provider := testutil.CreateProvider(t, cacheState, papp, pacct, keyto, nonce)

	deployment, groups := testutil.CreateDeployment(t, cacheState, dapp, tacct, keyfrom, nonce)
	group := groups.Items[0]

	order := testutil.CreateOrder(t, cacheState, oapp, tacct, keyfrom, deployment.Address, group.Seq, group.Seq)
	testutil.CreateFulfillment(t, cacheState, fapp, provider.Address, keyto, deployment.Address, group.Seq, order.Seq, price)
	lease := testutil.CreateLease(t, cacheState, lapp, provider.Address, keyto, deployment.Address, group.Seq, order.Seq, price)

	app.Commit()

	pacct, err = commitState.Account().Get(addrto)
	require.NoError(t, err)
	assert.Equal(t, uint64(lease.Price), pacct.Balance)

	tacct, err = commitState.Account().Get(addrfrom)
	require.NoError(t, err)
	assert.Equal(t, uint64(balance-lease.Price), tacct.Balance)

	app.Commit()

	pacct, err = commitState.Account().Get(addrto)
	require.NoError(t, err)
	assert.Equal(t, uint64(lease.Price)*2, pacct.Balance)

	tacct, err = commitState.Account().Get(addrfrom)
	require.NoError(t, err)
	assert.Equal(t, uint64(balance-lease.Price*2), tacct.Balance)

}
