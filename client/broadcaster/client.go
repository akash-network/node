package broadcaster

import (
	"context"
	"errors"
	"fmt"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	// ErrBroadcastTx is used when a broadcast fails due to tendermint errors
	ErrBroadcastTx = errors.New("broadcast tx error")
)

type Client interface {
	Broadcast(ctx context.Context, msgs ...sdk.Msg) error
}

type simpleClient struct {
	cctx sdkclient.Context
	txf  tx.Factory
	info keyring.Info
}

func NewClient(cctx sdkclient.Context, txf tx.Factory, info keyring.Info) Client {
	return &simpleClient{
		cctx: cctx,
		txf:  txf,
		info: info,
	}
}

func (c *simpleClient) Broadcast(_ context.Context, msgs ...sdk.Msg) error {
	txf, err := tx.PrepareFactory(c.cctx, c.txf)
	if err != nil {
		return err
	}
	return doBroadcast(c.cctx, txf, c.info.GetName(), msgs...)
}

func doBroadcast(cctx sdkclient.Context, txf tx.Factory, keyName string, msgs ...sdk.Msg) error {
	txn, err := tx.BuildUnsignedTx(txf, msgs...)
	if err != nil {
		return err
	}

	err = tx.Sign(txf, keyName, txn)
	if err != nil {
		return err
	}

	bytes, err := cctx.TxConfig.TxEncoder()(txn.GetTx())
	if err != nil {
		return err
	}

	response, err := cctx.BroadcastTxSync(bytes)
	if err != nil {
		return err
	}

	if response.Code != 0 {
		return fmt.Errorf("%w: response code %d - (%#v)", ErrBroadcastTx, response.Code, response)
	}

	return nil
}
