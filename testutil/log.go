package testutil

import (
	"os"

	"github.com/go-kit/kit/log/term"
	"github.com/tendermint/tendermint/libs/log"
)

func Logger() log.Logger {
	return log.NewTMLoggerWithColorFn(log.NewSyncWriter(os.Stdout), func(keyvals ...interface{}) term.FgBgColor {
		return term.FgBgColor{}
	})
}
