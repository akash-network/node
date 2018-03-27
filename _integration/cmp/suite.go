package cmp

import (
	"github.com/ovrclk/gestalt"
	g "github.com/ovrclk/gestalt/builder"
)

func Suite() gestalt.Component {
	key := newKey("master")
	return g.Suite("main").
		Run(GroupKeyCreate(key)).
		Run(GroupNodeRun(key)).
		Run(GroupAccountSend(key)).
		Run(GroupProviderRun(key))
}
