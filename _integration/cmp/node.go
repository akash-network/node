package cmp

import (
	"github.com/ovrclk/gestalt"
	g "github.com/ovrclk/gestalt/builder"
)

func NodeInit(key key) gestalt.Component {
	return Akashd("init", key.addr.Var()).
		WithMeta(g.Require(key.addr.Name()))
}

func NodeRun() gestalt.Component {
	return g.Group("run").
		Run(g.BG().
			Run(Akashd("start"))).
		Run(g.Retry(5).
			Run(Akash("status")))
}

func GroupNodeRun(key key) gestalt.Component {
	return g.Group("node-run").
		Run(NodeInit(key)).
		Run(NodeRun())
}
