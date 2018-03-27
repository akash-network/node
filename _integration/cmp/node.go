package cmp

import (
	"github.com/ovrclk/gestalt"
	g "github.com/ovrclk/gestalt/builder"
)

func nodeInit(key key) gestalt.Component {
	return akashd("node-init", "init", key.addr.Var()).
		WithMeta(g.Require(key.addr.Name()))
}

func nodeRun() gestalt.Component {
	check := akash("node-status", "status")

	return g.Group("node-run").
		Run(g.BG().
			Run(akashd("node-start", "start"))).
		Run(g.Retry(10).
			Run(check))
}

func groupNodeRun(key key) gestalt.Component {
	return g.Group("node").
		Run(nodeInit(key)).
		Run(nodeRun())
}
