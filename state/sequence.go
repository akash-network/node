package state

import "encoding/binary"

type Sequence interface {
	Current() uint64
	Advance() uint64
	Next() uint64
}

func NewSequence(db DB, path []byte) Sequence {
	return sequence{
		db:   db,
		path: path,
	}
}

type sequence struct {
	db   DB
	path []byte
}

func (s sequence) Current() uint64 {
	buf := s.db.Get(s.path)
	if buf == nil {
		return 0
	}
	return binary.BigEndian.Uint64(buf)
}

func (s sequence) Next() uint64 {
	return s.Current() + 1
}

func (s sequence) Advance() uint64 {
	next := s.Next()
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, next)
	s.db.Set(s.path, buf)
	return next
}
