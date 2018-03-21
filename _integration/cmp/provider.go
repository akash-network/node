package cmp

import (
	"github.com/ovrclk/gestalt"
	g "github.com/ovrclk/gestalt/builder"
	"github.com/ovrclk/gestalt/exec/js"
	"github.com/ovrclk/gestalt/vars"
)

func providerCreate(key vars.Ref, paddr vars.Ref) gestalt.Component {
	return g.Group("provider-create").
		Run(
			akash("provider", "create", "unused.yml", "-k", key.Name()).
				FN(g.Capture(paddr.Name())).
				WithMeta(g.Export(paddr.Name()))).
		Run(
			akash("query", "provider", paddr.Var()).
				FN(js.Do(js.Str(paddr.Var(), "address")))).
		WithMeta(g.Export(paddr.Name()))
}

func providerRun(key vars.Ref, paddr vars.Ref) gestalt.Component {
	return g.Group("provider-run").
		Run(akash("provider", "run", paddr.Var(), "-k", key.Name())).
		WithMeta(g.Require(paddr.Name()))
}

func groupProvider(key vars.Ref, paddr vars.Ref) gestalt.Component {
	return g.Group("provider").
		Run(providerCreate(key, paddr)).
		Run(g.BG().
			Run(providerRun(key, paddr))).
		WithMeta(g.Export(paddr.Name()))
}
