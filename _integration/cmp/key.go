package cmp

import (
	"github.com/ovrclk/gestalt"
	g "github.com/ovrclk/gestalt/builder"
	gx "github.com/ovrclk/gestalt/exec"
)

func KeyCreate(key key) gestalt.Component {
	return Akash("key", "create", key.name.Name()).
		FN(gx.Capture(key.addr.Name())).
		WithMeta(g.Export(key.addr.Name()))
}

func KeyList(key key) gestalt.Component {
	return Akash("key", "list").
		FN(gx.ParseColumns("name", "address").
			GrepField("name", key.name.Name()).
			GrepField("address", key.addr.Var()).
			EnsureCount(1).
			Done()).
		WithMeta(g.Require(key.addr.Name()))
}

func GroupKeyCreate(key key) gestalt.Component {
	return g.Group("key-create").
		Run(KeyCreate(key)).
		Run(KeyList(key)).
		WithMeta(g.Export(key.addr.Name()))
}
