package rest

import (
	"bytes"
	"context"
	"encoding/binary"
	"github.com/gorilla/websocket"
	"github.com/tendermint/tendermint/libs/log"
	"io"
	"k8s.io/client-go/tools/remotecommand"
	"sync"
	"time"
)

func leaseShellPingHandler(ctx context.Context, wg *sync.WaitGroup, ws *websocket.Conn) {
	defer wg.Done()
	pingTicker := time.NewTicker(pingPeriod)
	defer pingTicker.Stop()

	for {
		select {
		case <-pingTicker.C:
			const pingWriteWaitTime = 5 * time.Second
			if err := ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(pingWriteWaitTime)); err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func leaseShellWebsocketHandler(log log.Logger, wg *sync.WaitGroup, shellWs *websocket.Conn, stdinPipeOut io.Writer, terminalSizeUpdate chan<- remotecommand.TerminalSize) {
	defer wg.Done()
	for {
		shellWs.SetPongHandler(func(string) error {
			return shellWs.SetReadDeadline(time.Now().Add(pingWait))
		})

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
