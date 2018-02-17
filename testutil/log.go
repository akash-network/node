package testutil

import (
	"os"

	"github.com/tendermint/tmlibs/log"
)

func Logger() log.Logger {
	return log.NewTMLogger(log.NewSyncWriter(os.Stdout))
}
