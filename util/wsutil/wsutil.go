package wsutil

import (
	"bytes"
	"github.com/gorilla/websocket"
	"sync"
)


type WsWriterWrapper struct {
	connection *websocket.Conn

	id byte
	buf bytes.Buffer
	l sync.Locker
}

func NewWsWriterWrapper(conn *websocket.Conn, id byte, l sync.Locker) WsWriterWrapper {
	return WsWriterWrapper {
		connection: conn,
		l: l,
		id: id,
	}
}

func (wsw WsWriterWrapper) Write(data []byte) (int,error) {
	myBuf := &wsw.buf
	myBuf.Reset()
	_ = myBuf.WriteByte(wsw.id)
	_, _ = myBuf.Write(data)

	wsw.l.Lock()
	defer wsw.l.Unlock()
	err := wsw.connection.WriteMessage(websocket.BinaryMessage, myBuf.Bytes())

	return len(data), err
}