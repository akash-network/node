package wsutil

import (
	"bytes"
	"io"
	"sync"

	"github.com/gorilla/websocket"
)

// This type exposes the single method that this wrapper uses
type wrappedConnection interface {
	WriteMessage(int, []byte) error
}

type wsWriterWrapper struct {
	connection wrappedConnection

	id  byte
	buf bytes.Buffer
	l   sync.Locker
}

func NewWsWriterWrapper(conn wrappedConnection, id byte, l sync.Locker) io.Writer {
	return &wsWriterWrapper{
		connection: conn,
		l:          l,
		id:         id,
	}
}

func (wsw *wsWriterWrapper) Write(data []byte) (int, error) {
	myBuf := &wsw.buf
	myBuf.Reset()
	_ = myBuf.WriteByte(wsw.id)
	_, _ = myBuf.Write(data)

	wsw.l.Lock()
	defer wsw.l.Unlock()
	err := wsw.connection.WriteMessage(websocket.BinaryMessage, myBuf.Bytes())

	return len(data), err
}
