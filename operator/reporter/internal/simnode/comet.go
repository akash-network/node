package simnode

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	errorsmod "cosmossdk.io/errors"
	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/crypto/ed25519"
	cmtbytes "github.com/cometbft/cometbft/libs/bytes"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/p2p"
	cmtversion "github.com/cometbft/cometbft/proto/tendermint/version"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	rpcserver "github.com/cometbft/cometbft/rpc/jsonrpc/server"
	rpctypes "github.com/cometbft/cometbft/rpc/jsonrpc/types"
	cmttypes "github.com/cometbft/cometbft/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

// Versions the node reports, as an akash node on CometBFT 0.38 does.
const (
	cometVersion  = "0.38.26"
	p2pProtocol   = 8
	blockProtocol = 11
	appName       = "akash"
	listenAddr    = "tcp://0.0.0.0:26656"
	nodeMoniker   = "simnode"
	txIndexOn     = "on"
	txIndexOff    = "off"
)

var (
	validatorKey = ed25519.GenPrivKeyFromSecret([]byte("simnode validator")).PubKey()
	// consensusChannels are the reactor channels a CometBFT 0.38 validator advertises.
	consensusChannels = cmtbytes.HexBytes{0x40, 0x20, 0x21, 0x22, 0x23, 0x30, 0x38, 0x60, 0x61, 0x00}
)

// cometHandler serves the CometBFT JSON-RPC methods the reporters use, through CometBFT's own JSON-RPC server.
func (n *Node) cometHandler() http.Handler {
	mux := http.NewServeMux()
	rpcserver.RegisterRPCFuncs(mux, map[string]*rpcserver.RPCFunc{
		"status":            rpcserver.NewRPCFunc(n.status, ""),
		"abci_info":         rpcserver.NewRPCFunc(n.abciInfo, ""),
		"block":             rpcserver.NewRPCFunc(n.block, "height"),
		"abci_query":        rpcserver.NewRPCFunc(n.abciQuery, "path,data,height,prove"),
		"broadcast_tx_sync": rpcserver.NewRPCFunc(n.broadcastTxSync, "tx"),
		"tx":                rpcserver.NewRPCFunc(n.tx, "hash,prove"),
	}, log.NewNopLogger())

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if delay(r.Context(), n.opts.AkashLatency) != nil {
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func digest(kind string, h int64) cmtbytes.HexBytes {
	sum := sha256.Sum256([]byte("simnode akash " + kind + " " + strconv.FormatInt(h, 10)))
	return sum[:]
}

func blockID(h int64) cmttypes.BlockID {
	if h <= 0 {
		return cmttypes.BlockID{}
	}

	return cmttypes.BlockID{Hash: digest("block", h), PartSetHeader: cmttypes.PartSetHeader{Total: 1, Hash: digest("parts", h)}}
}

func (n *Node) nodeInfo() p2p.DefaultNodeInfo {
	index := txIndexOn
	if n.sc.Akash.NoTxIndex {
		index = txIndexOff
	}

	return p2p.DefaultNodeInfo{
		ProtocolVersion: p2p.NewProtocolVersion(p2pProtocol, blockProtocol, 0),
		DefaultNodeID:   p2p.ID(n.sc.Akash.NodeID),
		ListenAddr:      listenAddr,
		Network:         n.sc.Akash.ChainID,
		Version:         cometVersion,
		Channels:        consensusChannels,
		Moniker:         nodeMoniker,
		Other:           p2p.DefaultNodeInfoOther{TxIndex: index, RPCAddress: n.CometURL},
	}
}

func (n *Node) status(*rpctypes.Context) (*ctypes.ResultStatus, error) {
	n.count("status")
	n.mu.RLock()
	defer n.mu.RUnlock()

	h := n.servedHeight()
	t, _ := n.akashBlockTime(h)
	earliest, _ := n.akashBlockTime(n.ak.earliest)

	return &ctypes.ResultStatus{
		NodeInfo: n.nodeInfo(),
		SyncInfo: ctypes.SyncInfo{
			LatestBlockHash:     digest("block", h),
			LatestAppHash:       digest("app", h),
			LatestBlockHeight:   h,
			LatestBlockTime:     t,
			EarliestBlockHash:   digest("block", n.ak.earliest),
			EarliestAppHash:     digest("app", n.ak.earliest),
			EarliestBlockHeight: n.ak.earliest,
			EarliestBlockTime:   earliest,
			CatchingUp:          n.sc.Akash.CatchingUp,
		},
		ValidatorInfo: ctypes.ValidatorInfo{Address: validatorKey.Address(), PubKey: validatorKey},
	}, nil
}

func (n *Node) abciInfo(*rpctypes.Context) (*ctypes.ResultABCIInfo, error) {
	n.count("abci_info")
	n.mu.RLock()
	defer n.mu.RUnlock()

	app := n.appHeight()

	return &ctypes.ResultABCIInfo{Response: abci.ResponseInfo{
		Data:             appName,
		Version:          cometVersion,
		LastBlockHeight:  app,
		LastBlockAppHash: digest("app", app),
	}}, nil
}

func (n *Node) block(_ *rpctypes.Context, heightPtr *int64) (*ctypes.ResultBlock, error) {
	n.count("block")
	n.mu.RLock()
	defer n.mu.RUnlock()

	served := n.servedHeight()
	h := served
	if heightPtr != nil {
		h = *heightPtr
	}

	if h > served {
		return nil, fmt.Errorf("height %d must be less than or equal to the current blockchain height %d", h, served)
	}

	t, ok := n.akashBlockTime(h)
	if !ok || h < n.ak.earliest {
		return nil, fmt.Errorf("height %d is not available, lowest height is %d", h, n.ak.earliest)
	}

	txs := make(cmttypes.Txs, len(n.ak.blocks[h]))
	for i, raw := range n.ak.blocks[h] {
		txs[i] = raw
	}

	block := &cmttypes.Block{
		Header: cmttypes.Header{
			Version:            cmtversion.Consensus{Block: blockProtocol},
			ChainID:            n.sc.Akash.ChainID,
			Height:             h,
			Time:               t,
			LastBlockID:        blockID(h - 1),
			LastCommitHash:     digest("commit", h-1),
			DataHash:           txs.Hash(),
			ValidatorsHash:     digest("validators", 0),
			NextValidatorsHash: digest("validators", 0),
			ConsensusHash:      digest("consensus", 0),
			AppHash:            digest("app", h-1),
			LastResultsHash:    digest("results", h-1),
			EvidenceHash:       digest("evidence", h),
			ProposerAddress:    validatorKey.Address(),
		},
		Data:       cmttypes.Data{Txs: txs},
		LastCommit: &cmttypes.Commit{Height: h - 1, BlockID: blockID(h - 1)},
	}

	return &ctypes.ResultBlock{BlockID: blockID(h), Block: block}, nil
}

func (n *Node) abciQuery(_ *rpctypes.Context, path string, data cmtbytes.HexBytes, height int64, _ bool) (*ctypes.ResultABCIQuery, error) {
	n.count("abci_query")
	n.mu.RLock()
	defer n.mu.RUnlock()

	return &ctypes.ResultABCIQuery{Response: *n.query(path, data, height)}, nil
}

// query answers a gRPC query path as BaseApp's query router does (baseapp/abci.go handleQueryGRPC); the caller holds
// n.mu for reading.
func (n *Node) query(path string, data []byte, height int64) *abci.ResponseQuery {
	handler, ok := n.queries[path]
	if !ok {
		return sdkerrors.QueryResult(errorsmod.Wrapf(sdkerrors.ErrUnknownRequest, "unknown query path %s", path), false)
	}

	// BaseApp.Query answers an unpinned query at the last block height and echoes that height (baseapp/abci.go:162-163).
	h, err := n.queryHeight(height)
	if err != nil {
		return sdkerrors.QueryResult(err, false)
	}

	t, _ := n.akashBlockTime(h)
	res, err := handler(withView(context.Background(), view{height: h, time: t}), data)
	if err != nil {
		r := sdkerrors.QueryResult(grpcErrorToSDK(err), false)
		r.Height = h

		return r
	}

	return &abci.ResponseQuery{Height: h + n.sc.Akash.EchoOffset, Value: res}
}

func (n *Node) broadcastTxSync(_ *rpctypes.Context, raw cmttypes.Tx) (*ctypes.ResultBroadcastTx, error) {
	n.count("broadcast_tx_sync")

	res, abort, err := n.admit(raw)
	if abort {
		// Drop the connection without a response; net/http closes it quietly for this panic value.
		panic(http.ErrAbortHandler)
	}

	return res, err
}

// admit applies the next broadcast fault, then CometBFT's cache check and CheckTx. abort means the connection must
// drop without an answer.
func (n *Node) admit(raw cmttypes.Tx) (res *ctypes.ResultBroadcastTx, abort bool, err error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.ak.broadcasts = append(n.ak.broadcasts, append([]byte(nil), raw...))

	var fault BroadcastFault
	if len(n.sc.Akash.BroadcastFaults) > 0 {
		fault = n.sc.Akash.BroadcastFaults[0]
		n.sc.Akash.BroadcastFaults = n.sc.Akash.BroadcastFaults[1:]
	}

	tx := decodeTx(raw)
	reply := func(r execResult) *ctypes.ResultBroadcastTx {
		return &ctypes.ResultBroadcastTx{Code: r.code, Codespace: r.codespace, Log: r.log, Hash: raw.Hash()}
	}

	switch fault.Kind {
	case FaultTransport:
		return nil, true, nil
	case FaultInCache:
		return nil, false, errInCache
	case FaultNonceTaken:
		key := replayKey{timeout: tx.timeout.UnixNano()}
		if len(tx.signers) > 0 {
			key.signer = string(tx.signers[0])
		}
		space, code, log := errorsmod.ABCIInfo(nonceError(key), false)

		return reply(execResult{code: code, codespace: space, log: log}), false, nil
	case FaultReject:
		return reply(execResult{code: fault.Code, codespace: fault.Codespace, log: fault.Log}), false, nil
	}

	r, err := n.checkTx(tx)
	if err != nil {
		return nil, fault.Kind == FaultLostResponse, err
	}

	return reply(r), fault.Kind == FaultLostResponse, nil
}

func (n *Node) tx(_ *rpctypes.Context, hash []byte, _ bool) (*ctypes.ResultTx, error) {
	n.count("tx")
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.sc.Akash.NoTxIndex {
		return nil, errors.New("transaction indexing is disabled")
	}

	r := n.ak.results[string(hash)]
	if r == nil || r.height > n.servedHeight() {
		return nil, fmt.Errorf("tx (%X) not found", hash)
	}

	return &ctypes.ResultTx{
		Hash:   hash,
		Height: r.height,
		Index:  r.index,
		TxResult: abci.ExecTxResult{
			Code:      r.result.code,
			Codespace: r.result.codespace,
			Log:       r.result.log,
			GasWanted: r.result.gasWanted,
			GasUsed:   r.result.gasUsed,
		},
		Tx: r.raw,
	}, nil
}
