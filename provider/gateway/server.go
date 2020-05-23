package gateway

import (
	"context"
	"net"
	"net/http"

	"github.com/ovrclk/akash/provider"
	"github.com/tendermint/tendermint/libs/log"
)

func NewServer(ctx context.Context, log log.Logger, pclient provider.Client, address string) *http.Server {
	return &http.Server{
		Addr:    address,
		Handler: newRouter(log, pclient),
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}
}
