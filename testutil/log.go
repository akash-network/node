package testutil

import (
	"os"
	"testing"

	"github.com/tendermint/tendermint/libs/log"
)

func Logger(_ testing.TB) log.Logger {
	return log.NewTMLogger(os.Stderr)
}
