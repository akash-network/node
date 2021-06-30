package rest

import (
	"bytes"
	"encoding/binary"
	"github.com/gorilla/websocket"
	"github.com/tendermint/tendermint/libs/log"
	"io"
	"k8s.io/client-go/tools/remotecommand"
	"sync"
)

func leaseShellWebsocketHandler(log log.Logger, wg *sync.WaitGroup, shellWs *websocket.Conn, stdinPipeOut io.Writer, terminalSizeUpdate chan<- remotecommand.TerminalSize) {
	defer wg.Done()
	for {
		msgType, data, err := shellWs.ReadMessage()
		if err != nil {
			return
		}
		// Just ignore anything not a binary message or that is empty
		if msgType != websocket.BinaryMessage || len(data) == 0 {
			continue
		}

		msgID := data[0]
		msg := data[1:]
		switch msgID {
		case LeaseShellCodeStdin:
			_, err := stdinPipeOut.Write(msg)
			if err != nil {
				return
			}
		case LeaseShellCodeTerminalResize:
			var size remotecommand.TerminalSize
			r := bytes.NewReader(msg)
			// Unpack data, its just binary encoded data in big endian
			err = binary.Read(r, binary.BigEndian, &size.Width)
			if err != nil {
				return
			}
			err = binary.Read(r, binary.BigEndian, &size.Height)
			if err != nil {
				return
			}

			log.Debug("terminal resize received", "width", size.Width, "height", size.Height)
			if terminalSizeUpdate != nil {
				terminalSizeUpdate <- size
			}
		default:
			log.Error("unknown message ID on websocket", "code", msgID)
			return
		}

	}
}
