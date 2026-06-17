package types //nolint: revive

import (
	"context"
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

type TestParams struct {
	Home           string
	Node           string
	GRPC           string // host:port of the node's gRPC server (validator 0)
	SourceDir      string
	ChainID        string
	KeyringBackend string
	From           string
	FromAddress    sdk.AccAddress
}

type TestWorker interface {
	Run(ctx context.Context, t *testing.T, params TestParams)
}

var (
	preUpgradeWorkers  = map[string]TestWorker{}
	postUpgradeWorkers = map[string]TestWorker{}

	// universalPostUpgradeWorker, if set, runs after every upgrade regardless of
	// the upgrade name — used by the exhaustive gRPC tx/query suite which must
	// verify the API surface on every upgrade.
	universalPostUpgradeWorker TestWorker
)

// RegisterUniversalPostUpgradeWorker registers a worker that runs after every
// upgrade, in addition to any name-specific worker.
func RegisterUniversalPostUpgradeWorker(worker TestWorker) {
	if universalPostUpgradeWorker != nil {
		panic("universal post-upgrade worker already registered")
	}
	universalPostUpgradeWorker = worker
}

// GetUniversalPostUpgradeWorker returns the universal post-upgrade worker, if any.
func GetUniversalPostUpgradeWorker() TestWorker {
	return universalPostUpgradeWorker
}

func RegisterPreUpgradeWorker(name string, worker TestWorker) {
	if _, exists := preUpgradeWorkers[name]; exists {
		panic(fmt.Sprintf("pre-upgrade worker for upgrade \"%s\" already exists", name))
	}

	preUpgradeWorkers[name] = worker
}

func RegisterPostUpgradeWorker(name string, worker TestWorker) {
	if _, exists := postUpgradeWorkers[name]; exists {
		panic(fmt.Sprintf("post-upgrade worker for upgrade \"%s\" already exists", name))
	}

	postUpgradeWorkers[name] = worker
}

func GetPreUpgradeWorker(name string) TestWorker {
	return preUpgradeWorkers[name]
}

func GetPostUpgradeWorker(name string) TestWorker {
	return postUpgradeWorkers[name]
}
