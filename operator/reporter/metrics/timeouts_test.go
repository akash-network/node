package metrics

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestServeClosesStalledConnections(t *testing.T) {
	saved := serverTimeouts
	serverTimeouts = timeouts{
		header: 200 * time.Millisecond,
		read:   200 * time.Millisecond,
		write:  time.Second,
		idle:   200 * time.Millisecond,
	}

	t.Cleanup(func() { serverTimeouts = saved })

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := l.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { done <- Serve(ctx, l, prometheus.NewRegistry()) }()

	t.Cleanup(func() {
		cancel()

		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(2 * shutdownTimeout):
			t.Error("Serve did not return after cancel")
		}
	})

	for name, request := range map[string]string{
		"withheld body":   "POST /metrics HTTP/1.1\r\nHost: m\r\nContent-Length: 1\r\n\r\n",
		"idle keep-alive": "GET /metrics HTTP/1.1\r\nHost: m\r\n\r\n",
	} {
		t.Run(name, func(t *testing.T) {
			c, err := net.Dial("tcp", addr)
			require.NoError(t, err)

			defer func() { _ = c.Close() }()

			_, err = io.WriteString(c, request)
			require.NoError(t, err)

			require.NoError(t, c.SetReadDeadline(time.Now().Add(5*time.Second)))

			_, err = io.Copy(io.Discard, c) // the server may answer, but must then close
			require.NoError(t, err, "the connection stayed open")
		})
	}
}
