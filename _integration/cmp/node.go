package cmp

import (
	"github.com/ovrclk/gestalt"
	g "github.com/ovrclk/gestalt/builder"
)

func nodeInit(key key) gestalt.Component {
	return g.Group("node-init").
		Run(
			akashd("init", key.addr.Var())).
		WithMeta(g.Require(key.addr.Name()))
}

func nodeRun() gestalt.Component {
	return g.Group("node-run").
		Run(g.BG().
			Run(akashd("start"))).
		Run(g.Retry(10).
			Run(akash("status")))
}

func groupNodeRun(key key) gestalt.Component {
	return g.Group("node").
		Run(nodeInit(key)).
		Run(nodeRun())
}
