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

	check := akash("check", "query", "deployment", daddr.Var()).
		FN(parse)

	return g.Group("deploy-create").
		Run(
			akash("create", "deployment", "create", "{{deployment-path}}", "-k", key.Name()).
				FN(g.Capture(daddr.Name())).
				WithMeta(g.Export(daddr.Name()))).
		Run(g.Retry(5).Run(check)).
		WithMeta(g.Export(daddr.Name()).Require("deployment-path"))
}

func deployClose(key vars.Ref, daddr vars.Ref) gestalt.Component {
	check := deployQueryState(daddr, types.Deployment_CLOSED)

	return g.Group("deploy-close").
		Run(
			akash("close", "deployment", "close", daddr.Var(), "-k", key.Name()).
				WithMeta(g.Require(daddr.Name()))).
		Run(g.Retry(5).Run(check)).
		WithMeta(g.Require(daddr.Name()))
}

func deployQueryState(daddr vars.Ref, state types.Deployment_DeploymentState) gestalt.Component {
	parse := js.Do(
		js.Str(daddr.Var(), "address"),
		js.Int(int64(state), "state"),
	)

	return akash("deploy-query", "query", "deployment", daddr.Var()).
		FN(parse).
		WithMeta(g.Require(daddr.Name()))
}

func orderQuery(daddr vars.Ref) gestalt.Component {
	parse := js.Do(
		js.Str(daddr.Var(), "items", "[0]", "id", "deployment"),
	)

	return akash("order-query", "query", "order").
		FN(parse).
		WithMeta(g.Require(daddr.Name()))
}

func leaseQuery(daddr vars.Ref) gestalt.Component {
	parse := js.Do(
		js.Str(daddr.Var(), "items", "[0]", "id", "deployment"),
	)

	return akash("lease-query", "query", "lease").
		FN(parse).
		WithMeta(g.Require(daddr.Name()))
}

func groupDeploy(key vars.Ref, daddr vars.Ref) gestalt.Component {
	return g.Group("deployment").
		Run(deployCreate(key, daddr)).
		Run(g.Retry(5).
			Run(orderQuery(daddr))).
		Run(g.Retry(15).
			Run(leaseQuery(daddr))).
		Run(deployClose(key, daddr)).
		WithMeta(g.Export(daddr.Name()))
}
