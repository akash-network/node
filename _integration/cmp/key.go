package cmp

import (
	"github.com/ovrclk/gestalt"
	g "github.com/ovrclk/gestalt/builder"
	gx "github.com/ovrclk/gestalt/exec"
)

func keyCreate(key key) gestalt.Component {
	return akash("key-create", "key", "create", key.name.Name()).
		FN(gx.Capture(key.addr.Name())).
		WithMeta(g.Export(key.addr.Name()))
}

func keyList(key key) gestalt.Component {

	parse := gx.ParseColumns("name", "address").
		GrepField("name", key.name.Name()).
		GrepField("address", key.addr.Var()).
		EnsureCount(1).
		Done()

	return akash("key-list", "key", "list").
		FN(parse).
		WithMeta(g.Require(key.addr.Name()))
}

func groupKey(key key) gestalt.Component {
	return g.Group("key").
		Run(keyCreate(key)).
		Run(keyList(key)).
		WithMeta(g.Export(key.addr.Name()))
}
