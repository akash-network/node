package cmp

import (
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/gestalt"
	g "github.com/ovrclk/gestalt/builder"
	"github.com/ovrclk/gestalt/exec/js"
	"github.com/ovrclk/gestalt/vars"
)

func deployCreate(key vars.Ref, daddr vars.Ref) gestalt.Component {
	parse := js.Do(
		js.Str(daddr.Var(), "address"),
	)

	return g.Group("deploy-create").
		Run(
			akash("deployment", "create", "unused.yml", "-k", key.Name()).
				FN(g.Capture(daddr.Name())).
				WithMeta(g.Export(daddr.Name()))).
		Run(
			g.Retry(5).
				Run(
					akash("query", "deployment", daddr.Var()).
						FN(parse))).
		WithMeta(g.Export(daddr.Name()))
}

func deployClose(key vars.Ref, daddr vars.Ref) gestalt.Component {
	return g.Group("deploy-close").
		Run(
			akash("deployment", "close", daddr.Var(), "-k", key.Name()).
				WithMeta(g.Require(daddr.Name()))).
		Run(g.Retry(5).
			Run(deployQueryState(daddr, types.Deployment_CLOSED))).
		WithMeta(g.Require(daddr.Name()))
}

func deployQueryState(daddr vars.Ref, state types.Deployment_DeploymentState) gestalt.Component {
	parse := js.Do(
		js.Str(daddr.Var(), "address"),
		js.Int(int64(state), "state"),
	)
	return g.Group("deploy-query").
		Run(
			akash("query", "deployment", daddr.Var()).
				FN(parse).
				WithMeta(g.Require(daddr.Name())))
}

func orderQuery(daddr vars.Ref) gestalt.Component {
	parse := js.Do(
		js.Str(daddr.Var(), "items", "[0]", "deployment"),
	)
	return g.Group("order-query").
		Run(
			akash("query", "order").
				FN(parse)).
		WithMeta(g.Require(daddr.Name()))
}

func leaseQuery(daddr vars.Ref) gestalt.Component {
	parse := js.Do(
		js.Str(daddr.Var(), "items", "[0]", "deployment"),
	)
	return g.Group("lease-query").
		Run(
			akash("query", "lease").
				FN(parse)).
		WithMeta(g.Require(daddr.Name()))
}

func groupDeploy(key vars.Ref, daddr vars.Ref) gestalt.Component {
	return g.Group("deployment").
		Run(deployCreate(key, daddr)).
		Run(g.Retry(5).
			Run(orderQuery(daddr))).
		Run(g.Retry(10).
			Run(leaseQuery(daddr))).
		Run(deployClose(key, daddr)).
		WithMeta(g.Export(daddr.Name()))
}
