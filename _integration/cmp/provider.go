package cmp

import (
	"github.com/ovrclk/akash/_integration/js"
	"github.com/ovrclk/gestalt"
	g "github.com/ovrclk/gestalt/builder"
	"github.com/ovrclk/gestalt/vars"
)

func ProviderCreate(key key, paddr vars.Ref) gestalt.Component {
	return Akash("provider", "create", "unused.yml", "-k", key.name.Name()).
		FN(g.Capture(paddr.Name())).
		WithMeta(g.Export(paddr.Name()))
}

func ProviderQuery(paddr vars.Ref) gestalt.Component {
	return Akash("query", "provider", paddr.Var()).
		FN(js.PathEQStr(paddr.Var(), "address")).
		WithMeta(g.Require(paddr.Name()))
}

func ProviderRun(paddr vars.Ref) gestalt.Component {
	return g.Group("provider-run").
		Run(Akash("provider", "run", paddr.Var()))
}

func GroupProviderCreate(key key, paddr vars.Ref) gestalt.Component {
	return g.Group("provider-create").
		Run(ProviderCreate(key, paddr)).
		Run(ProviderQuery(paddr)).
		WithMeta(g.Export(paddr.Name()))
}

func GroupProviderRun(key key) gestalt.Component {
	paddr := g.Ref("provider-id")
	return g.Group("provider").
		Run(GroupProviderCreate(key, paddr)).
		Run(g.BG().
			Run(ProviderRun(paddr)))
}
