package cmp

import (
	"github.com/ovrclk/akash/_integration/js"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/gestalt"
	g "github.com/ovrclk/gestalt/builder"
	"github.com/ovrclk/gestalt/vars"
)

func DeployCreate(key vars.Ref, daddr vars.Ref) gestalt.Component {
	parse := js.Do(
		js.Str(daddr.Var(), "address"),
	)

	return g.Group("deploy-create").
		Run(
			Akash("deployment", "create", "unused.yml", "-k", key.Name()).
				FN(g.Capture(daddr.Name())).
				WithMeta(g.Export(daddr.Name()))).
		Run(
			g.Retry(5).
				Run(
					Akash("query", "deployment", daddr.Var()).
						FN(parse))).
		WithMeta(g.Export(daddr.Name()))
}

func DeployClose(key vars.Ref, daddr vars.Ref) gestalt.Component {
	return Akash("deployment", "close", daddr.Var(), "-k", key.Name()).
		WithMeta(g.Require(daddr.Name()))
}

func DeployQuery(daddr vars.Ref, state types.Deployment_DeploymentState) gestalt.Component {

	parse := js.Do(
		js.Str(daddr.Var(), "address"),
		js.Int(int64(state), "state"),
	)

	return Akash("query", "deployment", daddr.Var()).
		FN(parse).
		WithMeta(g.Export(daddr.Var()))
}

func OrderQuery(daddr vars.Ref) gestalt.Component {
	return Akash("query", "order").
		FN(js.Do(js.Str(daddr.Var(), "items", "[0]", "deployment"))).
		WithMeta(g.Require(daddr.Name()))
}

func LeaseQuery(daddr vars.Ref) gestalt.Component {
	return Akash("query", "lease").
		FN(js.Do(js.Str(daddr.Var(), "items", "[0]", "deployment"))).
		WithMeta(g.Require(daddr.Name()))
}

func GroupDeployCreate(key vars.Ref, daddr vars.Ref) gestalt.Component {
	return g.Group("deployment").
		Run(DeployCreate(key, daddr)).
		Run(g.Retry(5).
			Run(OrderQuery(daddr))).
		Run(g.Retry(10).
			Run(LeaseQuery(daddr))).
		Run(DeployClose(key, daddr)).
		Run(g.Retry(5).
			Run(DeployQuery(daddr, types.Deployment_CLOSED))).
		WithMeta(g.Export(daddr.Name()))
}
