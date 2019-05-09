package session

import (
	"fmt"
)

type ModeType string

const (
	ModeTypeInteractive ModeType = "interactive"
	ModeTypeText                 = "text"
	ModeTypeJSON                 = "json"
)

type runF func() error

type Mode interface {
	// Type must return the type of Mode
	Type() ModeType

	// When registers the events to run for the
	// current Mode when Run is invoked. It returns the current Mode
	When(ModeType, runF) Mode

	// Run runs the functions
	Run() error

	// Ask returns an Asker
	Ask() Asker

	// IsInteractive returns true if the current ModeType is ModeTypeInteractive
	IsInteractive() bool
}

type mode struct {
	modeType ModeType
	runners  []runF
	asker    Asker
}

func NewMode(mtype ModeType) (*mode, error) {
	switch mtype {
	case ModeTypeInteractive, ModeTypeText, ModeTypeJSON:
		return &mode{modeType: mtype}, nil
	default:
		return nil, fmt.Errorf("invalid interaction mode: %s", mtype)
	}
	return nil, nil
}

func (m *mode) Type() ModeType {
	return m.modeType
}

func (m *mode) When(mtype ModeType, fn runF) Mode {
	if mtype == m.modeType {
		m.runners = append(m.runners, fn)
	}
	return m
}

func (m *mode) Ask() Asker {
	if m.asker == nil {
		m.asker = NewInteractiveAsker(m.Type())
	}
	return m.asker
}

func (m *mode) Run() error {
	for i := 0; i < len(m.runners); i++ {
		var fn runF
		fn, m.runners = m.runners[0], m.runners[1:]
		if err := fn(); err != nil {
			return err
		}
	}
	return nil
}

func (m *mode) IsInteractive() bool {
	return m.modeType == ModeTypeInteractive
}
