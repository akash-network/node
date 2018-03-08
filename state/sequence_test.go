package state_test

import (
	"testing"

	"github.com/ovrclk/photon/state"
	"github.com/stretchr/testify/assert"
)

func TestSequence(t *testing.T) {
	db := state.NewMemDB()
	seq := state.NewSequence(db, []byte("/foo"))

	assert.Equal(t, uint64(0), seq.Current())
	assert.Equal(t, uint64(1), seq.Advance())
	assert.Equal(t, uint64(1), seq.Current())

	{
		seq := state.NewSequence(db, []byte("/foo"))
		assert.Equal(t, uint64(1), seq.Current())
	}

	{
		seq := state.NewSequence(db, []byte("/bar"))
		assert.Equal(t, uint64(0), seq.Current())
	}
}
