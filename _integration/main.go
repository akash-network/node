package main

import (
	"os"

	"github.com/ovrclk/gestalt"
	g "github.com/ovrclk/gestalt/builder"
	gx "github.com/ovrclk/gestalt/exec"
	"github.com/ovrclk/gestalt/vars"
)

func Node() gestalt.Component {
	return g.Group("node").
		Run(g.SH("cleanup", "echo", "cleanup")).
		Run(g.BG().
			Run(g.SH("start", "while true; do echo .; sleep 1; done"))).
		Run(g.SH("wait", "sleep", "10")).
		Run(g.Retry(5).
			Run(g.SH("check", "echo", "check")))
}

func Akash(args ...string) gx.Cmd {
	cmd := g.EXEC("akash", "{{akash-path}}", append([]string{"-d", "{{akash-root}}"}, args...)...)

	//cmd.AddEnv("AKASH_ROOT", "{{akash-root}}")

	cmd.WithMeta(g.Require("akash-path", "akash-root"))
	return cmd
}

func Akashd(args ...string) gx.Cmd {
	cmd := g.EXEC("akashd", "{{akashd-path}}", append([]string{"-d", "{{akashd-root}}"}, args...)...)

	//cmd.AddEnv("AKASHD_ROOT", "{{akashd-root}}")

	cmd.WithMeta(g.Require("akashd-path", "akashd-root"))
	return cmd
}

func CreateKey(name string) gestalt.Component {
	kname := name + "-addr"
	return Akash("key", "create", name).
		FN(gx.Capture(kname)).
		WithMeta(g.Export(kname))
}

func KeyList(name string) gestalt.Component {
	addr := vars.NewRef(name + "-addr")
	return Akash("key", "list").
		FN(gx.ParseColumns("name", "address").
			GrepField("name", name).
			GrepField("address", addr.Var()).
			EnsureCount(1).
			Done()).
		WithMeta(g.Require(addr.Name()))
}

func KeySuite() gestalt.Component {
	return g.Group("keys").
		Run(CreateKey("master")).
		Run(KeyList("master")).
		WithMeta(g.Export("master-addr"))
}

func NodeInit(name string) gestalt.Component {
	addr := vars.NewRef(name + "-addr")
	return Akashd("init", addr.Var()).
		WithMeta(g.Require(addr.Name()))
}

func NodeRun(name string) gestalt.Component {
	return g.Group("run").
		Run(g.BG().
			Run(Akashd("start"))).
		Run(g.Retry(5).
			Run(Akash("status")))
}

func NodeSuite() gestalt.Component {
	return g.Group("node").
		Run(NodeInit("master")).
		Run(NodeRun("master"))
}

func Suite() gestalt.Component {
	return g.Suite("main").
		Run(KeySuite()).
		Run(NodeSuite())
}

func main() {
	m := detectDefaults()

	suite := Suite()

	gestalt.RunWith(suite.WithMeta(m), os.Args[1:])
}

func detectDefaults() vars.Meta {
	return g.
		Default("akash-path", "../akash").
		Default("akash-root", "./data/client").
		Default("akashd-path", "../akashd").
		Default("akashd-root", "./data/node")
}
