package testutil

import (
	"sync"
	"testing"

	"github.com/cometbft/cometbft/libs/log"
)

func Logger(t testing.TB) log.Logger {
	return log.NewTMLogger(&testWriter{TB: t})
}

// Source: https://git.sr.ht/~samwhited/testlog/tree/b1b3e8e82fd6990e91ce9d0fbcbe69ac2d9b1f98/testlog.go
type testWriter struct {
	testing.TB
	lock sync.Mutex
}

func (tw *testWriter) Write(p []byte) (int, error) {
	defer tw.lock.Unlock()
	tw.lock.Lock()

	tw.Helper()
	tw.Logf("%s", p)
	return len(p), nil
}
