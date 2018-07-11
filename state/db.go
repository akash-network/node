package state

import (
	"sync"

	"github.com/tendermint/iavl"
)

type DBReader interface {
	IsEmpty() bool
	Version() int64
	Hash() []byte
	Get(key []byte) []byte
	GetRange([]byte, []byte, int) ([][]byte, [][]byte, error)
}

type DB interface {
	DBReader
	Commit() ([]byte, int64, error)
	Set(key, val []byte)
	Remove(key []byte)
}

type iavlDB struct {
	tree *iavl.MutableTree
	mtx  *sync.RWMutex
}

func (db *iavlDB) IsEmpty() bool {
	db.mtx.Lock()
	defer db.mtx.Unlock()
	return db.tree.IsEmpty()
}

func (db *iavlDB) Version() int64 {
	db.mtx.Lock()
	defer db.mtx.Unlock()
	return db.tree.Version()
}

func (db *iavlDB) Hash() []byte {
	db.mtx.Lock()
	defer db.mtx.Unlock()
	return db.tree.Hash()
}

func (db *iavlDB) Commit() ([]byte, int64, error) {
	db.mtx.Lock()
	defer db.mtx.Unlock()
	return db.tree.SaveVersion()
}

func (db *iavlDB) Get(key []byte) []byte {
	db.mtx.Lock()
	defer db.mtx.Unlock()
	_, val := db.tree.Get(key)
	return val
}

func (db *iavlDB) Set(key []byte, val []byte) {
	db.mtx.Lock()
	defer db.mtx.Unlock()
	db.tree.Set(key, val)
}

func (db *iavlDB) Remove(key []byte) {
	db.mtx.Lock()
	defer db.mtx.Unlock()
	db.tree.Remove(key)
}

func (db *iavlDB) GetRange(startKey []byte, endKey []byte, limit int) ([][]byte, [][]byte, error) {
	db.mtx.Lock()
	defer db.mtx.Unlock()
	keys, deps, _, err := db.tree.GetRangeWithProof(startKey, endKey, MaxRangeLimit)
	return keys, deps, err
}
