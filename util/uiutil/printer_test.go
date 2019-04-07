package uiutil

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrinter(t *testing.T) {
	var buf bytes.Buffer
	w := io.MultiWriter(&buf)
	p := NewPrinter(w)
	p.Add("foo bar")
	err := p.Flush()
	require.NoError(t, err)
}
