package rest

import (
	"bytes"
	"context"
	"github.com/stretchr/testify/require"
	"io"
	"k8s.io/client-go/tools/remotecommand"
	"strings"
	"sync"
	"testing"
)

func TestProcessRemoteErrorReturnsNoError(t *testing.T) {
	reader := strings.NewReader(`{"exit_code":0}`)
	err := processRemoteError(reader)
	require.NoError(t, err)
}

func TestProcessRemoteErrorReturnsErrorforExitCode(t *testing.T) {
	reader := strings.NewReader(`{"exit_code":1}`)
	err := processRemoteError(reader)
	require.Error(t, err)
	require.ErrorIs(t, err, errLeaseShell)
}

func TestProcessRemoteErrorReturnsErrorforMessage(t *testing.T) {
	reader := strings.NewReader(`{"exit_code":0, "message": "bob"}`)
	err := processRemoteError(reader)
	require.Error(t, err)
	require.ErrorIs(t, err, errLeaseShell)
}

func TestHandleStdinCopiesData(t *testing.T) {
	const testMsg = "testing"
	reader := strings.NewReader(testMsg)
	writer := &bytes.Buffer{}
	ctx, cancel := context.WithCancel(context.Background())
	saveError := func(string, error) {}

	handleStdin(ctx, reader, writer, saveError)
	cancel()

	require.Equal(t, writer.String(), testMsg)
}

func TestHandleStdinHalts(t *testing.T) {
	const testMsg = "testing"
	reader := strings.NewReader(testMsg)
	pipeIn, pipeOut := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	saveError := func(string, error) {}

	cancel()
	// Context is closed, so this just returns
	handleStdin(ctx, reader, pipeOut, saveError)

	require.NoError(t, pipeOut.Close())
	data, err := io.ReadAll(pipeIn)
	require.NoError(t, err)
	require.Equal(t, 0, len(data)) // Nothing writted by handleStdin
	require.NoError(t, pipeIn.Close())
}

func TestHandleTerminalResize(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	saveError := func(string, error) {}
	resizes := make(chan remotecommand.TerminalSize, 1)
	output := &bytes.Buffer{}

	const testWidth = 100
	const testHeight = 0xffff - 3
	resizes <- remotecommand.TerminalSize{
		Width:  testWidth,
		Height: testHeight,
	}
	close(resizes)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	// Halts immediately because the channel is closed
	handleTerminalResize(ctx, wg, resizes, output, saveError)
	cancel()

	data, err := io.ReadAll(output)
	require.NoError(t, err)
	require.Equal(t, data, []byte{0x0, 100, 0xff, 0xff - 3})
}

func TestHandleTerminalResizeHalts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	saveError := func(string, error) {}
	resizes := make(chan remotecommand.TerminalSize, 1)
	output := &bytes.Buffer{}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	cancel()
	// Halts immediately because the context is closed
	handleTerminalResize(ctx, wg, resizes, output, saveError)

	data, err := io.ReadAll(output)
	require.NoError(t, err)
	require.Len(t, data, 0)
}
