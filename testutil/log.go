package testutil

import (
	"testing"

	"github.com/tendermint/tendermint/libs/log"
)

func Logger(t testing.TB) log.Logger {
	return log.NewTMLogger(testWriter{t})
}

// Source: https://git.sr.ht/~samwhited/testlog/tree/b1b3e8e82fd6990e91ce9d0fbcbe69ac2d9b1f98/testlog.go
type testWriter struct {
	testing.TB
}

func (tw testWriter) Write(p []byte) (int, error) {
	tw.Helper()
	tw.Logf("%s", p)
	return len(p), nil
}
