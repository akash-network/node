package cmp

import (
	g "github.com/ovrclk/gestalt/builder"
	gx "github.com/ovrclk/gestalt/exec"
)

func akash(name string, args ...string) gx.Cmd {
	cmd := g.EXEC("akash-"+name,
		"{{akash-path}}",
		append([]string{"-d", "{{akash-root}}"}, args...)...)

	cmd.WithMeta(g.Require("akash-path", "akash-root"))
	return cmd
}

func akashd(name string, args ...string) gx.Cmd {
	cmd := g.EXEC("akashd-"+name,
		"{{akashd-path}}",
		append([]string{"-d", "{{akashd-root}}"}, args...)...)
	cmd.WithMeta(g.Require("akashd-path", "akashd-root"))
	return cmd
}
