package cmp

import (
	"github.com/ovrclk/gestalt"
	g "github.com/ovrclk/gestalt/builder"
)

func Suite() gestalt.Component {
	key := newKey("master")
	paddr := g.Ref("provider-id")
	daddr := g.Ref("deployment-id")
	return g.Suite("main").
		Run(groupKey(key)).
		Run(groupNodeRun(key)).
		Run(groupAccountSend(key)).
		Run(groupProvider(paddr)).
		Run(groupDeploy(key.name, daddr))
}
