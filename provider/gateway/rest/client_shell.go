package rest

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/ovrclk/akash/util/wsutil"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"io"
	"k8s.io/client-go/tools/remotecommand"
	"net/url"
	"sync"
)

var ErrLeaseShellProviderError = errors.New("the provider encountered an unknown error")

func (c *client) LeaseShell(ctx context.Context, lID mtypes.LeaseID, service string, podIndex uint, cmd []string,
	stdin io.ReadCloser,
	stdout io.Writer,
	stderr io.Writer,
	tty bool,
	terminalResize <-chan remotecommand.TerminalSize) error {

	endpoint, err := url.Parse(c.host.String() + "/" + leaseShellPath(lID))
	if err != nil {
		return err
	}

	switch endpoint.Scheme {
	case schemeWSS, schemeHTTPS:
		endpoint.Scheme = schemeWSS
	default:
		return fmt.Errorf("invalid uri scheme %q", endpoint.Scheme)
	}

	query := url.Values{}
	query.Set("service", service)
	query.Set("podIndex", fmt.Sprintf("%d", podIndex))
	ttyValue := "0"
	if tty {
		ttyValue = "1"
	}
	query.Set("tty", ttyValue)

	stdinValue := "0"
	if stdin != nil {
		stdinValue = "1"
	}
	query.Set("stdin", stdinValue)

	for i, v := range cmd {
		query.Set(fmt.Sprintf("cmd%d", i), v)
	}

	endpoint.RawQuery = query.Encode()
	subctx, subcancel := context.WithCancel(ctx)
	conn, response, err := c.wsclient.DialContext(subctx, endpoint.String(), nil)
	if err != nil {
		if errors.Is(err, websocket.ErrBadHandshake) {
			buf := &bytes.Buffer{}
			_, _ = io.Copy(buf, response.Body)

			return ClientResponseError{
				Status:  response.StatusCode,
				Message: buf.String(),
			}
		}
		return err
	}

	wg := &sync.WaitGroup{}
	suberr := make(chan error, 1)
	saveError := func(msg string, err error) {
		err = fmt.Errorf("%w: failed while" + msg)
		// The channel is buffered but do not block here
		select {
		case suberr <- err:
		default:
		}
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-subctx.Done()
		err := conn.Close()
		if err != nil {
			saveError("closing websocket", err)
		}
	}()

	l := &sync.Mutex{}

	if stdin != nil {
		// This goroutine is orphaned. There is no universal way to cancel a read from stdin
		// at this time
		go func() {
			data := make([]byte, 4096)

			writer := wsutil.NewWsWriterWrapper(conn, LeaseShellCodeStdin, l)
			for {
				n, err := stdin.Read(data)
				if err != nil {
					saveError("reading from stdin", err)
					return
				}

				select {
				case <-subctx.Done():
					return
				default:
				}

				_, err = writer.Write(data[0:n])
				if err != nil {
					saveError("writing stdin data to remote", err)
					return
				}
			}
		}()
	}

	if tty && terminalResize != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var w io.Writer
			w = wsutil.NewWsWriterWrapper(conn, LeaseShellCodeTerminalResize, l)
			buf := bytes.Buffer{}
			for {
				var size remotecommand.TerminalSize
				var ok bool
				select {
				case <-subctx.Done():
					return
				case size, ok = <-terminalResize:
					if !ok { // Channel has closed
						return
					}

				}

				// Clear the buffer, then pack in both values
				(&buf).Reset()
				err = binary.Write(&buf, binary.BigEndian, size.Width)
				if err != nil {
					saveError("encoding terminal size width", err)
					return
				}
				err = binary.Write(&buf, binary.BigEndian, size.Height)
				if err != nil {
					saveError("encoding terminal size height", err)
					return
				}

				_, err = w.Write((&buf).Bytes())
				if err != nil {
					saveError("sending terminal size to remote", err)
					return
				}
			}
		}()
	}

	var remoteError *bytes.Buffer
	var connectionError error
loop:
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if messageType != websocket.BinaryMessage {
			continue // Just ignore anything else
		}

		if len(data) == 0 {
			connectionError = errors.New("provider sent a message that is too short to parse")
		}

		msgId := data[0] // First byte is always message ID
		msg := data[1:]  // remainder is the message
		switch msgId {
		case LeaseShellCodeStdout:
			_, connectionError = stdout.Write(msg)
		case LeaseShellCodeStderr:
			_, connectionError = stderr.Write(msg)
		case LeaseShellCodeResult:
			remoteError = bytes.NewBuffer(msg)
			break loop
		case LeaseShellCodeFailure:
			connectionError = ErrLeaseShellProviderError
		default:
			connectionError = fmt.Errorf("provider sent unknown message ID %d", messageType)
		}

		if connectionError != nil {
			break loop
		}
	}

	subcancel()

	if stdin != nil {
		err := stdin.Close()
		if err != nil {
			saveError("closing stdin", err)
		}
	}

	if remoteError != nil {
		dec := json.NewDecoder(remoteError)
		var v leaseShellResponse
		err := dec.Decode(&v)
		if err != nil {
			return fmt.Errorf("%w: failed parsing response data from provider", err)
		}

		if 0 != len(v.Message) {
			return errors.New(v.Message)
		}

		if 0 != v.ExitCode {
			return fmt.Errorf("remote process exited with code %d", v.ExitCode)
		}

	}

	// Check to see if a goroutine failed
	select {
	case err = <-suberr:
		return err
	default:
	}

	wg.Wait()

	return connectionError
}
