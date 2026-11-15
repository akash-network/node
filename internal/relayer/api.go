package relayer

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	solana "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
	"pkg.akt.dev/node/v2/internal/relayer/native"
)

type BuildInfo struct {
	Version  string
	Commit   string
	Platform string
}

type Streams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

type stateHeight int64

type headerHeight int64

type dependencies struct {
	now       func() time.Time
	sleep     func(context.Context, time.Duration) error
	writeJSON func(string, []byte) error
	streams   Streams
}

type service struct {
	cfg        config
	deps       dependencies
	solana     *rpc.Client
	payer      solana.PrivateKey
	genesis    string
	store      *stateStore
	native     *native.Client
	reader     *bridge.Reader
	deployment bridge.Deployment
	closeHTTP  func()

	followed    *bridge.Followed
	access      bridge.Access
	prunedAt    int64
	headerRetry retryState
	unsupported retryState
	alerts      map[string]int64
	waiting     map[sink.ID]string
	eventMu     sync.Mutex

	expiryMu        sync.Mutex
	expiryCooldowns map[string]expiryCooldown
}

// A payer session is only created while holding the existing deployment lock.
type payerSession struct {
	service *service
}

type txRequest struct {
	stage        string
	instructions []solana.Instruction
	tables       map[solana.PublicKey]solana.PublicKeySlice
	preflight    bool
	submitted    func(string) error
}

type txResult struct {
	stage         string
	signature     string
	bytes         int
	accounts      int
	fee           uint64
	compute       uint64
	failure       json.RawMessage
	logs          []string
	payerLamports *int64
}

type observe func(txResult) error

type preparedProof struct {
	proof native.Proof
	err   error
}
