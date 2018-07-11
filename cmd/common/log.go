package common

import (
	"io"

	"github.com/go-kit/kit/log/term"
	"github.com/tendermint/tendermint/libs/log"
)

func NewLogger(w io.Writer) log.Logger {
	return log.NewTMLoggerWithColorFn(log.NewSyncWriter(w), logColorFn)
}

func logColorFn(keyvals ...interface{}) term.FgBgColor {
	return term.FgBgColor{}
}
