package main

import (
	"os"

	"github.com/ovrclk/gestalt"
	g "github.com/ovrclk/gestalt/builder"
	gx "github.com/ovrclk/gestalt/exec"
	"github.com/ovrclk/gestalt/vars"
)

type key struct {
	name vars.Ref
	addr vars.Ref
}

func newKey(name string) key {
	return key{
		name: vars.NewRef(name),
		addr: vars.NewRef(name + "-addr"),
	}
}

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
	cmd := g.EXEC("akash",
		"{{akash-path}}",
		append([]string{"-d", "{{akash-root}}"}, args...)...)

	cmd.WithMeta(g.Require("akash-path", "akash-root"))
	return cmd
}

func Akashd(args ...string) gx.Cmd {
	cmd := g.EXEC("akashd",
		"{{akashd-path}}",
		append([]string{"-d", "{{akashd-root}}"}, args...)...)

	cmd.WithMeta(g.Require("akashd-path", "akashd-root"))
	return cmd
}

func CreateKey(key key) gestalt.Component {
	return Akash("key", "create", key.name.Name()).
		FN(gx.Capture(key.addr.Name())).
		WithMeta(g.Export(key.addr.Name()))
}

func KeyList(key key) gestalt.Component {
	return Akash("key", "list").
		FN(gx.ParseColumns("name", "address").
			GrepField("name", key.name.Name()).
			GrepField("address", key.addr.Var()).
			EnsureCount(1).
			Done()).
		WithMeta(g.Require(key.addr.Name()))
}

func KeySuite(key key) gestalt.Component {
	return g.Group("keys").
		Run(CreateKey(key)).
		Run(KeyList(key)).
		WithMeta(g.Export(key.addr.Name()))
}

func NodeInit(key key) gestalt.Component {
	return Akashd("init", key.addr.Var()).
		WithMeta(g.Require(key.addr.Name()))
}

func NodeRun() gestalt.Component {
	return g.Group("run").
		Run(g.BG().
			Run(Akashd("start"))).
		Run(g.Retry(5).
			Run(Akash("status")))
}

func NodeSuite(key key) gestalt.Component {
	return g.Group("node").
		Run(NodeInit(key)).
		Run(NodeRun())
}

func KeyBalance(key key, amount int) gestalt.Component {
	// check account balance
	return g.Group("account-balance")
}

func SendTo(from key, to key, amount int) gestalt.Component {
	// send `amount` from `from` to `to`
	return g.Group("account-send")
}

func SendAmount(key key) gestalt.Component {
	other := newKey("other")
	return g.Group("send").
		Run(KeySuite(other)).
		Run(KeyBalance(key, 10000)).
		Run(SendTo(key, other, 100)).
		Run(KeyBalance(key, 10000-100)).
		Run(KeyBalance(other, 100))
}

func Suite() gestalt.Component {
	key := newKey("master")
	return g.Suite("main").
		Run(KeySuite(key)).
		Run(NodeSuite(key)).
		Run(SendAmount(key))
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
