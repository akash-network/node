package state

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/ovrclk/photon/types"
	"github.com/tendermint/iavl"
	tmdb "github.com/tendermint/tmlibs/db"
)

const (
	dbCacheSize = 256
	dbBackend   = tmdb.GoLevelDBBackendStr
)

func LoadDB(pathname string) (DB, error) {

	if pathname == "" {
		return NewMemDB(), nil
	}

	pathname, err := filepath.Abs(pathname)
	if err != nil {
		return nil, err
	}

	pathname = strings.TrimSuffix(pathname, path.Ext(pathname))

	dir := path.Dir(pathname)
	name := path.Base(pathname)

	db := tmdb.NewDB(name, dbBackend, dir)
	tree := iavl.NewVersionedTree(dbCacheSize, db)
	if err := tree.Load(); err != nil {
		return nil, err
	}

	return &iavlDB{tree}, nil
}

func LoadState(db DB, gen *types.Genesis) (State, error) {

	state := NewState(db)

	if gen == nil || !db.IsEmpty() {
		return state, nil
	}

	accounts := state.Account()

	for idx := range gen.Accounts {
		if err := accounts.Save(&gen.Accounts[idx]); err != nil {
			return nil, err
		}
	}

	return state, nil
}

func NewMemDB() DB {
	tree := iavl.NewVersionedTree(0, tmdb.NewMemDB())
	return &iavlDB{tree}
}
