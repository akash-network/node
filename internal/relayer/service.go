package relayer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
	"pkg.akt.dev/node/v2/internal/relayer/native"
)

func defaultDependencies(streams Streams) dependencies {
	if streams.Out == nil {
		streams.Out = io.Discard
	}
	if streams.Err == nil {
		streams.Err = io.Discard
	}
	return dependencies{
		now:       time.Now,
		sleep:     sleep,
		writeJSON: atomicJSON,
		streams:   streams,
	}
}

func sleep(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func newService(ctx context.Context, cfg config, deps dependencies) (*service, error) {
	payer, err := loadPayer(cfg.payerPath)
	if err != nil {
		return nil, err
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	httpClient := &http.Client{
		Transport:     transport,
		Timeout:       30 * time.Second,
		CheckRedirect: native.RPCCheckRedirect,
	}
	endpoint, solanaHTTP, err := native.RPCEndpointHTTP(cfg.solanaRPC, httpClient)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, err
	}
	client := rpc.NewWithCustomRPCClient(jsonrpc.NewClientWithOpts(endpoint, &jsonrpc.RPCClientOpts{
		HTTPClient: solanaHTTP,
	}))

	genesis, err := client.GetGenesisHash(ctx)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, err
	}
	if cfg.genesis != nil && genesis != *cfg.genesis {
		transport.CloseIdleConnections()
		return nil, fmt.Errorf("Solana RPC serves another cluster: genesis hash differs from solanaGenesisHash")
	}

	nativeHTTP := &http.Client{
		Transport:     transport,
		Timeout:       15 * time.Second,
		CheckRedirect: native.RPCCheckRedirect,
	}
	node, err := native.NewWithHTTP(cfg.akashRPC, cfg.chainID, cfg.maxAge, nativeHTTP)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, err
	}

	deployment, err := bridge.Derive(cfg.program)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, err
	}

	store, err := newStateStore(cfg.stateDir, cfg.chainID, genesis.String(), cfg.program.String(), payer.PublicKey().String(), deps.writeJSON)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, err
	}

	return &service{
		cfg:        cfg,
		deps:       deps,
		solana:     client,
		payer:      payer,
		genesis:    genesis.String(),
		store:      store,
		native:     node,
		reader:     bridge.NewReader(client, cfg.program, cfg.chainID, cfg.pinnedClient),
		deployment: deployment,
		closeHTTP:  transport.CloseIdleConnections,
		alerts:     make(map[string]int64),
		waiting:    make(map[sink.ID]string),
	}, nil
}

func (s *service) close() {
	if s.closeHTTP != nil {
		s.closeHTTP()
	}
}

func (s *service) follow(ctx context.Context) (bridge.Followed, error) {
	found, err := s.reader.Follow(ctx)
	if err != nil {
		var mismatch *bridge.FollowMismatch
		if errors.As(err, &mismatch) && s.alertDue("follow") {
			fields := map[string]any{
				"event":         mismatch.Event,
				"lightClient":   mismatch.Found.Program.String(),
				"bridgeChainId": mismatch.Found.ChainID,
				"chainId":       s.cfg.chainID,
				"action":        "stopped",
			}
			if s.cfg.pinnedClient != nil {
				fields["pinned"] = s.cfg.pinnedClient.String()
			}
			s.emit(true, fields)
		}
		return bridge.Followed{}, err
	}
	s.clearAlert("follow")
	if s.followed == nil {
		s.emit(false, map[string]any{
			"event":         "light-client",
			"program":       found.Program.String(),
			"accessManager": found.AccessManager.String(),
			"chainId":       found.ChainID,
			"pinned":        s.cfg.pinnedClient != nil,
		})
	} else if s.followed.Program != found.Program {
		s.emit(true, map[string]any{
			"event":         "light-client-changed",
			"previous":      s.followed.Program.String(),
			"current":       found.Program.String(),
			"accessManager": found.AccessManager.String(),
			"slot":          found.Slot,
		})
		s.headerRetry = retryState{}
		s.unsupported = retryState{}
		s.prunedAt = 0
	}
	s.followed = &found
	return found, nil
}

func (s *service) relayerAccess(ctx context.Context, followed bridge.Followed) (bridge.Access, error) {
	access, err := s.reader.Access(ctx, followed, s.payer.PublicKey())
	if err != nil {
		return "", err
	}
	if s.access != access {
		s.emit(access == bridge.AccessNone, map[string]any{
			"event":       "relayer-access",
			"access":      access,
			"payer":       s.payer.PublicKey().String(),
			"lightClient": followed.Program.String(),
			"updates":     access != bridge.AccessNone,
			"prunes":      access == bridge.AccessMember,
		})
	}
	s.access = access
	return access, nil
}
