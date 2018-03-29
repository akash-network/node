package cmp

import (
	"github.com/ovrclk/gestalt"
	g "github.com/ovrclk/gestalt/builder"
	gx "github.com/ovrclk/gestalt/exec"
	"github.com/ovrclk/gestalt/vars"
)

func keyCreate(root vars.Ref, key key) gestalt.Component {
	return akash_(root, "key-create", "key", "create", key.name.Name()).
		FN(gx.Capture(key.addr.Name())).
		WithMeta(g.Export(key.addr.Name()))
}

func keyList(root vars.Ref, key key) gestalt.Component {

	parse := gx.ParseColumns("name", "address").
		GrepField("name", key.name.Name()).
		GrepField("address", key.addr.Var()).
		EnsureCount(1).
		Done()

	return akash_(root, "key-list", "key", "list").
		FN(parse).
		WithMeta(g.Require(key.addr.Name()))
}

func groupKey(key key) gestalt.Component {
	return groupKey_(defaultAkashRoot, key)
}

func groupKey_(root vars.Ref, key key) gestalt.Component {
	return g.Group("key").
		Run(keyCreate(root, key)).
		Run(keyList(root, key)).
		WithMeta(g.Export(key.addr.Name()))
}
