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
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	"io"
	"k8s.io/client-go/tools/remotecommand"
	"net/url"
	"sync"
)

var (
	errLeaseShell              = errors.New("lease shell failed")
	ErrLeaseShellProviderError = fmt.Errorf("%w: the provider encountered an unknown error", errLeaseShell)
)

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
		return fmt.Errorf("%w: invalid uri scheme %q", errLeaseShell, endpoint.Scheme)
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

			subcancel()
			return ClientResponseError{
				Status:  response.StatusCode,
				Message: buf.String(),
			}
		}
		subcancel()
		return err
	}

	wg := &sync.WaitGroup{}
	suberr := make(chan error, 1)
	saveError := func(msg string, err error) {
		err = fmt.Errorf("%w: failed while %s", err, msg)
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
		stdinWriter := wsutil.NewWsWriterWrapper(conn, LeaseShellCodeStdin, l)
		// This goroutine is orphaned. There is no universal way to cancel a read from stdin
		// at this time
		go handleStdin(subctx, stdin, stdinWriter, saveError)
	}

	if tty && terminalResize != nil {
		wg.Add(1)
		terminalOutput := wsutil.NewWsWriterWrapper(conn, LeaseShellCodeTerminalResize, l)
		go handleTerminalResize(subctx, wg, terminalResize, terminalOutput, saveError)
	}

	var remoteErrorData *bytes.Buffer
	var connectionError error
loop:
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			saveError("receiving from websocket", err)
			break
		}
		if messageType != websocket.BinaryMessage {
			continue // Just ignore anything else
		}

		if len(data) == 0 {
			connectionError = fmt.Errorf("%w: provider sent a message that is too short to parse", errLeaseShell)
		}

		msgID := data[0] // First byte is always message ID
		msg := data[1:]  // remainder is the message
		switch msgID {
		case LeaseShellCodeStdout:
			_, connectionError = stdout.Write(msg)
		case LeaseShellCodeStderr:
			_, connectionError = stderr.Write(msg)
		case LeaseShellCodeResult:
			remoteErrorData = bytes.NewBuffer(msg)
			break loop
		case LeaseShellCodeFailure:
			connectionError = ErrLeaseShellProviderError
		default:
			connectionError = fmt.Errorf("%w: provider sent unknown message ID %d", errLeaseShell, messageType)
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

	// Check to see if the remote end returned an error
	if remoteErrorData != nil {
		if err = processRemoteError(remoteErrorData); err != nil {
			return err
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

func processRemoteError(input io.Reader) error {
	dec := json.NewDecoder(input)
	var v leaseShellResponse
	err := dec.Decode(&v)
	if err != nil {
		return fmt.Errorf("%w: failed parsing response data from provider", err)
	}

	if 0 != len(v.Message) {
		return fmt.Errorf("%w: %s", errLeaseShell, v.Message)
	}

	if 0 != v.ExitCode {
		return fmt.Errorf("%w: remote process exited with code %d", errLeaseShell, v.ExitCode)
	}

	return nil
}

func handleStdin(ctx context.Context, input io.Reader, output io.Writer, saveError func(string, error)) {
	data := make([]byte, 4096)

	for {
		n, err := input.Read(data)
		if err != nil {
			saveError("reading from stdin", err)
			return
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		_, err = output.Write(data[0:n])
		if err != nil {
			saveError("writing stdin data to remote", err)
			return
		}
	}
}

func handleTerminalResize(ctx context.Context, wg *sync.WaitGroup, input <-chan remotecommand.TerminalSize, output io.Writer, saveError func(string, error)) {
	defer wg.Done()

	buf := &bytes.Buffer{}
	for {
		var size remotecommand.TerminalSize
		var ok bool
		select {
		case <-ctx.Done():
			return
		case size, ok = <-input:
			if !ok { // Channel has closed
				return
			}

		}

		// Clear the buffer, then pack in both values
		buf.Reset()
		err := binary.Write(buf, binary.BigEndian, size.Width)
		if err != nil {
			saveError("encoding terminal size width", err)
			return
		}
		err = binary.Write(buf, binary.BigEndian, size.Height)
		if err != nil {
			saveError("encoding terminal size height", err)
			return
		}

		_, err = output.Write((buf).Bytes())
		if err != nil {
			saveError("sending terminal size to remote", err)
			return
		}
	}
}
