package state

import (
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/akash/types"
	"github.com/tendermint/iavl"
	tmdb "github.com/tendermint/tendermint/libs/db"
)

const (
	dbCacheSize = 256
	dbBackend   = tmdb.GoLevelDBBackend
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
	tree := iavl.NewMutableTree(db, dbCacheSize)
	if _, err := tree.Load(); err != nil {
		return nil, err
	}

	mtx := new(sync.RWMutex)
	return &iavlDB{tree, mtx}, nil
}

func LoadState(db DB, gen *types.Genesis) (CommitState, CacheState, error) {

	commitState := NewState(db)
	cacheState := NewCache(db)

	if gen == nil || !db.IsEmpty() {
		return commitState, cacheState, nil
	}

	accounts := cacheState.Account()

	for idx := range gen.Accounts {
		if err := accounts.Save(&gen.Accounts[idx]); err != nil {
			return nil, nil, err
		}
	}

	if err := cacheState.Write(); err != nil {
		return nil, nil, err
	}

	return commitState, cacheState, nil
}

func NewMemDB() DB {
	mtx := new(sync.RWMutex)
	tree := iavl.NewMutableTree(tmdb.NewMemDB(), 0)
	return &iavlDB{tree, mtx}
}

// // save object writes an object to the base db
// func saveObject(db DB, key []byte, obj proto.Message) error {
// 	buf, err := proto.Marshal(obj)
// 	if err != nil {
// 		return err
// 	}

// 	db.Set(key, buf)
// 	return nil
// }

// save object writes an object to the state
func saveObject(state State, key []byte, obj proto.Message) error {
	buf, err := proto.Marshal(obj)
	if err != nil {
		return err
	}

	state.Set(key, buf)
	return nil
}
