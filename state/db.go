package state

import "github.com/tendermint/iavl"

type DB interface {
	IsEmpty() bool
	Version() uint64
	Hash() []byte
	Commit(version uint64) ([]byte, error)

	Get(key []byte) []byte
	Set(key, val []byte)
	Remove(key []byte) ([]byte, bool)
	GetWithProof(key []byte) ([]byte, iavl.KeyProof, error)
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
