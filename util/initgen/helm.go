package initgen

import (
	"io/ioutil"
	"os"
	"path"

	"github.com/ovrclk/akash/node"
	"github.com/tendermint/tendermint/p2p"
	yaml "gopkg.in/yaml.v2"
)

func NewMultiHelmWriter(ctx Context) Writer {
	return &multiHelmWriter{ctx: ctx}
}

type multiHelmWriter struct {
	ctx Context
}

func (w multiHelmWriter) Write() error {
	for _, node := range w.ctx.Nodes() {
		nctx := NewContext(node.Name, w.ctx.Path(), w.ctx.Genesis(), node)
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
	var nkey []byte
	var peers []helmNodePeer
	if len(w.ctx.Nodes()) > 0 {

		curNode := w.ctx.Nodes()[0]

		vbuf, err = node.FilePVToJSON(curNode.FilePV)
		if err != nil {
			return err
		}

		nkey, err = node.NodeKeyToJSON(curNode.NodeKey)
		if err != nil {
			return err
		}

		for _, node := range curNode.Peers {
			peers = append(peers, helmNodePeer{Name: node.Name, ID: node.NodeKey.ID()})
		}
	}

	obj := helmConfig{
		Node: helmNodeConfig{
			Name:    w.ctx.Name(),
			Genesis: string(gbuf),
			PVKey:   string(vbuf),
			NodeKey: string(nkey),
			Peers:   peers,
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

type helmNodeConfig struct {
	Name    string
	Genesis string
	PVKey   string `yaml:"priv_validator_key"`
	NodeKey string `yaml:"node_key"`
	Peers   []helmNodePeer
}

type helmNodePeer struct {
	Name string
	ID   p2p.ID
}

type helmConfig struct {
	Node helmNodeConfig
}
