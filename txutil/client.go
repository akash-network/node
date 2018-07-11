package txutil

import (
	"errors"
	"sync"

	"github.com/cosmos/cosmos-sdk/crypto/keys"
	"github.com/ovrclk/akash/query"
	"github.com/ovrclk/akash/types"
	tmclient "github.com/tendermint/tendermint/rpc/client"
	tmctypes "github.com/tendermint/tendermint/rpc/core/types"
)

type Client interface {
	Key() keys.Info
	Signer() Signer
	Nonce() (uint64, error)
	BroadcastTxCommit(tx interface{}) (*tmctypes.ResultBroadcastTxCommit, error)
}

func NewClient(parent tmclient.ABCIClient, signer Signer, key keys.Info, nonce uint64) Client {
	return &client{
		parent: parent,
		signer: signer,
		key:    key,
		nonce:  nonce,
	}
}

type client struct {
	parent tmclient.ABCIClient
	signer Signer
	key    keys.Info
	nonce  uint64
	mtx    sync.Mutex
}

func (c *client) Key() keys.Info {
	return c.key
}

func (c *client) Signer() Signer {
	return c.signer
}

func (c *client) Nonce() (uint64, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.currentNonce()
}

func (c *client) BroadcastTxCommit(itx interface{}) (*tmctypes.ResultBroadcastTxCommit, error) {
	nonce, err := c.nonceWithAdvance()

	if err != nil {
		return nil, err
	}

	tx, err := BuildTx(c.signer, nonce, itx)
	if err != nil {
		return nil, err
	}

	res, err := c.parent.BroadcastTxCommit(tx)
	if err != nil {
		return res, err
	}

	if !res.CheckTx.IsOK() {
		return res, errors.New(res.CheckTx.GetLog())
	}
	if !res.DeliverTx.IsOK() {
		return res, errors.New(res.DeliverTx.GetLog())
	}

	return res, nil
}

func (c *client) nonceWithAdvance() (uint64, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	nonce, err := c.currentNonce()
	if err == nil {
		c.nonce++
	}
	return nonce, nil
}

func (c *client) currentNonce() (uint64, error) {
	if c.nonce != 0 {
		return c.nonce, nil
	}
	path := query.AccountPath(c.key.GetPubKey().Address())
	result, err := c.parent.ABCIQuery(path, nil)
	if err != nil {
		return 0, err
	}
	res := new(types.Account)
	if err := res.Unmarshal(result.Response.Value); err != nil {
		return 0, err
	}
	c.nonce = res.Nonce + 1
	return c.nonce, nil
}
