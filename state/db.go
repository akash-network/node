package state

import "github.com/tendermint/iavl"

type DBReader interface {
	IsEmpty() bool
	Version() uint64
	Hash() []byte
	Get(key []byte) []byte
	GetWithProof(key []byte) ([]byte, iavl.KeyProof, error)
	GetRangeWithProof([]byte, []byte, int) ([][]byte, [][]byte, iavl.KeyRangeProof, error)
	String() string
}

type DB interface {
	DBReader
	Commit(version uint64) ([]byte, error)
	Set(key, val []byte)
	Remove(key []byte) ([]byte, bool)
}

type iavlDB struct {
	tree *iavl.VersionedTree
}

func (db *iavlDB) IsEmpty() bool {
	return db.tree.IsEmpty()
}

func (db *iavlDB) Version() uint64 {
	return db.tree.LatestVersion()
}

func (db *iavlDB) Hash() []byte {
	return db.tree.Hash()
}

func (db *iavlDB) Commit(version uint64) ([]byte, error) {
	return db.tree.SaveVersion(version)
}

func (db *iavlDB) Get(key []byte) []byte {
	_, val := db.tree.Get(key)
	return val
}

func (db *iavlDB) Set(key []byte, val []byte) {
	db.tree.Set(key, val)
}

func (db *iavlDB) Remove(key []byte) ([]byte, bool) {
	return db.tree.Remove(key)
}

func (db *iavlDB) GetWithProof(key []byte) ([]byte, iavl.KeyProof, error) {
	return db.tree.GetWithProof(key)
}

func (db *iavlDB) GetRangeWithProof(startKey []byte, endKey []byte, limit int) ([][]byte, [][]byte, iavl.KeyRangeProof, error) {
	keys, deps, proof, err := db.tree.GetRangeWithProof(startKey, endKey, limit)
	return keys, deps, *proof, err
}

func (db *iavlDB) String() string {
	return db.tree.String()
}
