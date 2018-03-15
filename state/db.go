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
	GetWithProof(key []byte) ([]byte, iavl.KeyProof, error)
	GetRangeWithProof([]byte, []byte, int) ([][]byte, [][]byte, iavl.KeyRangeProof, error)
}

type DB interface {
	DBReader
	Commit() ([]byte, int64, error)
	Set(key, val []byte)
	Remove(key []byte) ([]byte, bool)
}

type iavlDB struct {
	tree *iavl.VersionedTree
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
	return db.tree.Version64()
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

func (db *iavlDB) Remove(key []byte) ([]byte, bool) {
	db.mtx.Lock()
	defer db.mtx.Unlock()
	return db.tree.Remove(key)
}

func (db *iavlDB) GetWithProof(key []byte) ([]byte, iavl.KeyProof, error) {
	db.mtx.Lock()
	defer db.mtx.Unlock()
	return db.tree.GetWithProof(key)
}

func (db *iavlDB) GetRangeWithProof(startKey []byte, endKey []byte, limit int) ([][]byte, [][]byte, iavl.KeyRangeProof, error) {
	db.mtx.Lock()
	defer db.mtx.Unlock()
	keys, deps, proof, err := db.tree.GetRangeWithProof(startKey, endKey, limit)
	return keys, deps, *proof, err
}
