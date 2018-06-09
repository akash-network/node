package state

import (
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
)

type AccountAdapter interface {
	Save(account *types.Account) error
	Get(base.Bytes) (*types.Account, error)
	KeyFor(base.Bytes) base.Bytes
}

type accountAdapter struct {
	state State
}

func NewAccountAdapter(state State) AccountAdapter {
	return &accountAdapter{state}
}

func (a *accountAdapter) Save(account *types.Account) error {
	key := a.KeyFor(account.Address)
	return saveObject(a.state, key, account)
}

func (a *accountAdapter) Get(address base.Bytes) (*types.Account, error) {

	acc := types.Account{}

	key := a.KeyFor(address)

	buf := a.state.Get(key)
	if buf == nil {
		return nil, nil
	}

	if err := acc.Unmarshal(buf); err != nil {
		return nil, err
	}

	return &acc, nil
}

func (a *accountAdapter) KeyFor(address base.Bytes) base.Bytes {
	return append([]byte(AccountPath), address...)
}
