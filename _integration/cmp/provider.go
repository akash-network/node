package cmp

import (
	"github.com/ovrclk/akash/_integration/js"
	"github.com/ovrclk/gestalt"
	g "github.com/ovrclk/gestalt/builder"
	"github.com/ovrclk/gestalt/vars"
)

func ProviderCreate(key key, ref vars.Ref) gestalt.Component {
	return Akash("provider", "create", "unused.yml", "-k", key.name.Name()).
		FN(g.Capture(ref.Name())).
		WithMeta(g.Export(ref.Name()))
}

func ProviderQuery(addr vars.Ref) gestalt.Component {
	return Akash("query", "provider", addr.Var()).
		FN(js.PathEQStr(addr.Var(), "address")).
		WithMeta(g.Require(addr.Name()))
}

func GroupProviderCreate(key key) gestalt.Component {
	addr := vars.NewRef("provider-id")
	return g.Group("provider-create").
		Run(ProviderCreate(key, addr)).
		Run(ProviderQuery(addr)).
		WithMeta(g.Export(addr.Name()))
}
