package cmp

import (
	"github.com/ovrclk/gestalt"
	g "github.com/ovrclk/gestalt/builder"
	"github.com/ovrclk/gestalt/exec/js"
	"github.com/ovrclk/gestalt/vars"
)

func providerCreate(root vars.Ref, key vars.Ref, paddr vars.Ref) gestalt.Component {

	check := akash_(root, "query", "query", "provider", paddr.Var()).
		FN(js.Do(js.Str(paddr.Var(), "address")))

	return g.Group("provider-create").
		Run(
			akash_(root, "create", "provider", "create", "{{provider-path}}", "-k", key.Name()).
				FN(g.Capture(paddr.Name())).
				WithMeta(g.Export(paddr.Name()))).
		Run(g.Retry(5).Run(check)).
		WithMeta(g.Export(paddr.Name()).
			Require("provider-path"))
}

func providerRun(root vars.Ref, key vars.Ref, paddr vars.Ref) gestalt.Component {
	return akash_(root, "provider-run", "provider", "run", paddr.Var(), "-k", key.Name()).
		WithMeta(g.Require(paddr.Name()))
}

func groupProvider(paddr vars.Ref) gestalt.Component {
	root := g.Ref("provider-root")
	key := newKey("provider-master")
	return g.Group("provider").
		Run(groupKey_(root, key)).
		Run(providerCreate(root, key.name, paddr)).
		Run(g.BG().
			Run(providerRun(root, key.name, paddr))).
		WithMeta(g.Export(paddr.Name()))
}
