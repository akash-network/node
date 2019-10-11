package initgen

import (
	"fmt"
	"os"
	"path"

	"github.com/ovrclk/akash/node"
)

const (
	GenesisFilename = "genesis.json"
	PVStateFilename = "priv_validator_state.json"
	PVKeyFilename   = "priv_validator_key.json"
	NodeKeyFilename = "node_key.json"
	ConfigDir       = "config"
	DataDir         = "data"
)

func NewMultiDirWriter(ctx Context) Writer {
	return multiDirWriter{ctx: ctx}
}

type multiDirWriter struct {
	ctx Context
}

func (w multiDirWriter) Write() error {
	for _, node := range w.ctx.Nodes() {
		path := path.Join(w.ctx.Path(), node.Name)
		nctx := NewContext(path, w.ctx.Genesis(), node)
		nw := NewDirWriter(nctx)
		if err := nw.Write(); err != nil {
			return err
		}
	}
	return nil
}

func NewDirWriter(ctx Context) Writer {
	return dirWriter{ctx: ctx}
}

type dirWriter struct {
	ctx Context
}

func (w dirWriter) Write() error {
	if len(w.ctx.Nodes()) > 1 {
		return fmt.Errorf("%T: too many private validators", w)
	}

	if err := os.MkdirAll(w.basecfgdir(), 0755); err != nil {
		return err
	}

	if err := os.MkdirAll(w.basedatadir(), 0755); err != nil {
		return err
	}

	if len(w.ctx.Nodes()) > 0 {
		curNode := w.ctx.Nodes()[0]
		fpath := path.Join(w.basecfgdir(), PVKeyFilename)
		if err := node.PVKeyToFile(fpath, 0400, curNode.FilePV.Key); err != nil {
			return err
		}
		fpath = path.Join(w.basedatadir(), PVStateFilename)
		if err := node.PVStateToFile(fpath, 0400, curNode.FilePV.LastSignState); err != nil {
			return err
		}

		fpath = path.Join(w.basecfgdir(), NodeKeyFilename)
		if err := node.NodeKeyToFile(fpath, 0400, curNode.NodeKey); err != nil {
			return err
		}
	}

	fpath := path.Join(w.basecfgdir(), GenesisFilename)
	if err := w.ctx.Genesis().SaveAs(fpath); err != nil {
		return err
	}

	return nil
}

func (w dirWriter) basecfgdir() string {
	return path.Join(w.ctx.Path(), ConfigDir)
}

func (w dirWriter) basedatadir() string {
	return path.Join(w.ctx.Path(), DataDir)
}
