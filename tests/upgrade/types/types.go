package types

import (
	"context"
	"fmt"
	"testing"
)

type TestParams struct {
	Home           string
	Node           string
	ChainID        string
	KeyringBackend string
	From           string
}

type TestWorker interface {
	Run(ctx context.Context, t *testing.T, params TestParams)
}

var (
	preUpgradeWorkers  = map[string]TestWorker{}
	postUpgradeWorkers = map[string]TestWorker{}
)

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
