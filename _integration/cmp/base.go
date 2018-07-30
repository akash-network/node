package cmp

import (
	g "github.com/ovrclk/gestalt/builder"
	gx "github.com/ovrclk/gestalt/exec"
	"github.com/ovrclk/gestalt/vars"
)

var defaultAkashRoot = g.Ref("akash-root")

func akash(name string, args ...string) gx.Cmd {
	return akash_(defaultAkashRoot, name, args...)
}

func akashd(name string, args ...string) gx.Cmd {
	cmd := g.EXEC("akashd-"+name,
		"{{akashd-path}}",
		append([]string{"-d", "{{akashd-root}}"}, args...)...)
	cmd.WithMeta(g.Require("akashd-path", "akashd-root"))
	return cmd
}

func akash_(root vars.Ref, name string, args ...string) gx.Cmd {
	cmd := g.EXEC("akash-"+name,
		"{{akash-path}}",
		append([]string{"-d", root.Var()}, args...)...).
		AddEnv("AKASH_NODE", "{{akash-node}}")

	cmd.WithMeta(g.
		Require("akash-path", root.Name()).
		Default("akash-node", "http://localhost:46657"))
	return cmd
}
