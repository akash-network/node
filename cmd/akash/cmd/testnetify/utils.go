package testnetify

import (
	"context"
	"io"
	"os"
	"path/filepath"

	dbm "github.com/cosmos/cosmos-db"
	sdksrv "github.com/cosmos/cosmos-sdk/server"
	"golang.org/x/sync/errgroup"
	cflags "pkg.akt.dev/go/cli/flags"

	"pkg.akt.dev/node/util/server"
)

func openDB(rootDir string, backendType dbm.BackendType) (dbm.DB, error) {
	dataDir := filepath.Join(rootDir, "data")
	return dbm.NewDB("application", backendType, dataDir)
}

func setupTraceWriter(svrCtx *sdksrv.Context) (traceWriter io.WriteCloser, cleanup func(), err error) {
	// clean up the traceWriter when the server is shutting down
	cleanup = func() {}

	traceWriterFile := svrCtx.Viper.GetString(cflags.FlagTraceStore)
	traceWriter, err = openTraceWriter(traceWriterFile)
	if err != nil {
		return traceWriter, cleanup, err
	}

	// if flagTraceStore is not used, then traceWriter is nil
	if traceWriter != nil {
		cleanup = func() {
			if err = traceWriter.Close(); err != nil {
				svrCtx.Logger.Error("failed to close trace writer", "err", err)
			}
		}
	}

	return traceWriter, cleanup, nil
}

func openTraceWriter(traceWriterFile string) (w io.WriteCloser, err error) {
	if traceWriterFile == "" {
		return
	}
	return os.OpenFile( //nolint: gosec
		traceWriterFile,
		os.O_WRONLY|os.O_APPEND|os.O_CREATE,
		0o666,
	)
}

func getCtx(sctx *sdksrv.Context, block bool) (*errgroup.Group, context.Context) {
	ctx, cancelFn := context.WithCancel(context.Background())
	g, ctx := errgroup.WithContext(ctx)
	// listen for quit signals so the calling parent process can gracefully exit
	server.ListenForQuitSignals(ctx, cancelFn, g, block, sctx.Logger)

	return g, ctx
}
