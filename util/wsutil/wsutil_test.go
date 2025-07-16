package wsutil

import (
	"io"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type dummyConnection struct {
	messageType int
	data        []byte

	returns error
}

func (dc *dummyConnection) WriteMessage(mt int, data []byte) error {
	dc.messageType = mt
	dc.data = data
	return dc.returns
}

func TestWebsocketWriterWrapperWritesPrefix(t *testing.T) {
	const testID = 0xab
	l := &sync.Mutex{}
	conn := &dummyConnection{}

	wrapper := NewWsWriterWrapper(conn, testID, l)

	n, err := wrapper.Write([]byte{0x1, 0x2, 0x3})
	require.NoError(t, err)
	require.Equal(t, n, 3)

	require.Equal(t, conn.messageType, websocket.BinaryMessage)
	require.Equal(t, conn.data, []byte{testID, 0x1, 0x2, 0x3})
}

func TestWebsocketWriterWrapperReturnsError(t *testing.T) {
	const testID = 0xab
	l := &sync.Mutex{}
	conn := &dummyConnection{}
	conn.returns = io.EOF // Any error works

	wrapper := NewWsWriterWrapper(conn, testID, l)

	_, err := wrapper.Write([]byte{0x1, 0x2, 0x3})
	require.Error(t, err)
	require.Equal(t, io.EOF, err)
}
