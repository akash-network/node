package cmp

import "github.com/ovrclk/gestalt/vars"

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
