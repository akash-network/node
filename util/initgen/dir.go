package initgen

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
)

const (
	GenesisFilename          = "genesis.json"
	PrivateValidatorFilename = "priv_validator.json"
)

func NewMultiDirWriter(ctx Context) Writer {
	return multiDirWriter{ctx: ctx}
}

type multiDirWriter struct {
	ctx Context
}

func (w multiDirWriter) Write() error {
	for idx, pv := range w.ctx.PrivateValidators() {
		name := fmt.Sprintf("%v-%v", w.ctx.Name(), idx)
		path := path.Join(w.ctx.Path(), name)
		nctx := NewContext(name, path, w.ctx.Genesis(), pv)
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

	if len(w.ctx.PrivateValidators()) > 1 {
		return fmt.Errorf("%T: too many private validators", w)
	}

	if err := os.MkdirAll(w.ctx.Path(), 0755); err != nil {
		return err
	}

	if len(w.ctx.PrivateValidators()) > 0 {
		fpath := path.Join(w.ctx.Path(), PrivateValidatorFilename)
		if err := writeObj(fpath, 0400, w.ctx.PrivateValidators()[0]); err != nil {
			return err
		}
	}

	fpath := path.Join(w.ctx.Path(), GenesisFilename)
	if err := writeObj(fpath, 0644, w.ctx.Genesis()); err != nil {
		return err
	}

	return nil
}

func writeObj(path string, perm os.FileMode, obj interface{}) error {
	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}
	_, err = os.Stat(path)
	if !os.IsNotExist(err) {
		return nil
	}
	err = ioutil.WriteFile(path, data, perm)
	if err != nil {
		return err
	}
	return nil
}
