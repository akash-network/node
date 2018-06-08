package state

type CommitState interface {
	Version() int64
	Hash() []byte
	Commit() ([]byte, int64, error)

	DB() DBReader
}

func NewState(db DB) CommitState {
	return &commitState{db}
}

type commitState struct {
	db DB
}

func (s *commitState) Version() int64 {
	return s.db.Version()
}

func (s *commitState) Hash() []byte {
	return s.db.Hash()
}

func (s *commitState) Commit() ([]byte, int64, error) {
	return s.db.Commit()
}

func (s *commitState) DB() DBReader {
	return s.db
}