package main

import (
	"os"

	"github.com/ovrclk/akash/_integration/cmp"
	"github.com/ovrclk/gestalt"
	g "github.com/ovrclk/gestalt/builder"
	"github.com/ovrclk/gestalt/vars"
)

func main() {
	m := detectDefaults()

	suite := cmp.Suite()

	gestalt.RunWith(suite.WithMeta(m), os.Args[1:])
}

func detectDefaults() vars.Meta {
	return g.
		Default("akash-path", "../akash").
		Default("akash-root", "./data/client").
		Default("akashd-path", "../akashd").
		Default("akashd-root", "./data/node")
}
