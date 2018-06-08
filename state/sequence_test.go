package state_test

import (
	"testing"

	"github.com/ovrclk/akash/state"
	"github.com/stretchr/testify/assert"
)

func TestSequence(t *testing.T) {
	db := state.NewMemDB()
	st := state.NewState(db)
	seq := state.NewSequence(st, []byte("/foo"))

	assert.Equal(t, uint64(0), seq.Current())
	assert.Equal(t, uint64(1), seq.Advance())
	assert.Equal(t, uint64(1), seq.Current())

	{
		seq := state.NewSequence(st, []byte("/foo"))
		assert.Equal(t, uint64(1), seq.Current())
	}

	{
		seq := state.NewSequence(st, []byte("/bar"))
		assert.Equal(t, uint64(0), seq.Current())
	}
}
