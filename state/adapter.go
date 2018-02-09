package state

import (
	"errors"

	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	wire "github.com/tendermint/go-wire"
)

const (
	balanceSize = 8
	accountPath = "/accounts/"
)

type AccountAdapter interface {
	Save(account *types.Account) error
	Get(base.Bytes) (*types.Account, error)
}

type accountAdapter struct {
	db DB
}

func NewAccountAdapter(db DB) AccountAdapter {
	return &accountAdapter{db}
}

func (a *accountAdapter) Save(account *types.Account) error {
	key := append([]byte(accountPath), account.Address...)

	buf := make([]byte, balanceSize)
	wire.PutUint64(buf, account.GetBalance())

	a.db.Set(key, buf)
	return nil
}

func (a *accountAdapter) Get(address base.Bytes) (*types.Account, error) {
	key := append([]byte(accountPath), address...)

	buf := a.db.Get(key)

	if buf == nil {
		return nil, nil
	}

	if len(buf) != balanceSize {
		return nil, errors.New("invalid balance")
	}

	balance := wire.GetUint64(buf)

	return &types.Account{Address: address, Balance: balance}, nil
}
