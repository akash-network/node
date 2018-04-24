package initgen

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/ovrclk/akash/node"
	yaml "gopkg.in/yaml.v2"
)

func NewMultiHelmWriter(ctx Context) Writer {
	return &multiHelmWriter{ctx: ctx}
}

type multiHelmWriter struct {
	ctx Context
}

func (w multiHelmWriter) Write() error {
	for idx, pv := range w.ctx.PrivateValidators() {
		name := fmt.Sprintf("%v-%v", w.ctx.Name(), idx)
		nctx := NewContext(name, w.ctx.Path(), w.ctx.Genesis(), pv)
		nw := NewHelmWriter(nctx)
		if err := nw.Write(); err != nil {
			return err
		}
	}
	return nil
}

func NewHelmWriter(ctx Context) Writer {
	return &helmWriter{ctx: ctx}
}

type helmWriter struct {
	ctx Context
}

func (w helmWriter) Write() error {

	gbuf, err := node.TMGenesisToJSON(w.ctx.Genesis())
	if err != nil {
		return err
	}

	var vbuf []byte
	if len(w.ctx.PrivateValidators()) > 0 {
		vbuf, err = node.PVToJSON(w.ctx.PrivateValidators()[0])
		if err != nil {
			return err
		}
	}

	obj := HelmConfig{
		Node: HelmNodeConfig{
			Name:      w.ctx.Name(),
			Genesis:   string(gbuf),
			Validator: string(vbuf),
		},
	}

	buf, err := yaml.Marshal(obj)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(w.ctx.Path(), 0755); err != nil {
		return err
	}

	opath := path.Join(w.ctx.Path(), w.ctx.Name()+".yaml")

	return ioutil.WriteFile(opath, buf, 0644)
}

type HelmNodeConfig struct {
	Name      string
	Genesis   string
	Validator string `yaml:"priv_validator"`
}

type HelmConfig struct {
	Node HelmNodeConfig
}
