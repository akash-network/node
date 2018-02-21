package state

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
)

const (
	AccountPath = "/accounts/"
)

type AccountAdapter interface {
	Save(account *types.Account) error
	Get(base.Bytes) (*types.Account, error)
	KeyFor(base.Bytes) base.Bytes
}

type accountAdapter struct {
	db DB
}

func NewAccountAdapter(db DB) AccountAdapter {
	return &accountAdapter{db}
}

func (a *accountAdapter) Save(account *types.Account) error {
	key := a.KeyFor(account.Address)

	abytes, err := proto.Marshal(account)
	if err != nil {
		return err
	}

	a.db.Set(key, abytes)
	return nil
}

func (a *accountAdapter) Get(address base.Bytes) (*types.Account, error) {

	acc := types.Account{}

	key := a.KeyFor(address)

	buf := a.db.Get(key)
	if buf == nil {
		return nil, nil
	}

	acc.Unmarshal(buf)

	return &acc, nil
}

func (a *accountAdapter) KeyFor(address base.Bytes) base.Bytes {
	return append([]byte(AccountPath), address...)
}
