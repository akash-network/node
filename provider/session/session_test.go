package session

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSessionGetCreatedAt(t *testing.T) {
	const testCreatedAt = int64(13156)
	s := New(nil, nil, nil, testCreatedAt)
	assert.Equal(t, s.CreatedAtBlockHeight(), testCreatedAt)
}
