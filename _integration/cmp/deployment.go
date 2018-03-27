package cmp

import (
	"github.com/ovrclk/akash/_integration/js"
	"github.com/ovrclk/gestalt"
	g "github.com/ovrclk/gestalt/builder"
	"github.com/ovrclk/gestalt/vars"
)

func DeployCreate(key vars.Ref, daddr vars.Ref) gestalt.Component {
	return Akash("deployment", "create", "unused.yml", "-k", key.Name()).
		FN(g.Capture(daddr.Name())).
		WithMeta(g.Export(daddr.Name()))
}

func DeployQuery(daddr vars.Ref) gestalt.Component {
	return Akash("query", "deployment", daddr.Var()).
		FN(js.PathEQStr(daddr.Var(), "address")).
		WithMeta(g.Export(daddr.Var()))
}

func OrderQuery(daddr vars.Ref) gestalt.Component {
	return Akash("query", "order").
		FN(js.PathEQStr(daddr.Var(), "items", "[0]", "deployment")).
		WithMeta(g.Require(daddr.Name()))
}

func LeaseQuery(daddr vars.Ref) gestalt.Component {
	return Akash("query", "lease").
		FN(js.PathEQStr(daddr.Var(), "items", "[0]", "deployment")).
		WithMeta(g.Require(daddr.Name()))
}

func GroupDeployCreate(key vars.Ref, daddr vars.Ref) gestalt.Component {
	return g.Group("deploy-create").
		Run(DeployCreate(key, daddr)).
		Run(g.Retry(5).
			Run(DeployQuery(daddr))).
		Run(g.Retry(5).
			Run(OrderQuery(daddr))).
		Run(g.Retry(10).
			Run(LeaseQuery(daddr))).
		WithMeta(g.Export(daddr.Name()))
}
