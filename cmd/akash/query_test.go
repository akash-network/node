package main

import (
	"testing"

	"github.com/ovrclk/akash/cmd/akash/query"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/assert"
)

func TestAccountQuery_NoNode(t *testing.T) {
	hexaddr := testutil.HexAddress(t)
	args := []string{query.QueryCommand().Name(), "account", hexaddr}
	base := baseCommand()
	base.AddCommand(query.QueryCommand())
	base.SetArgs(args)
	err := base.Execute()
	assert.Error(t, err)
}

func TestDeploymentQuery_NoNode(t *testing.T) {
	hexaddr := testutil.HexDeploymentAddress(t)
	args := []string{query.QueryCommand().Name(), "deployment", hexaddr}
	base := baseCommand()
	base.AddCommand(query.QueryCommand())
	base.SetArgs(args)
	err := base.Execute()
	assert.Error(t, err)
}

func TestOrderQuery_NoNode(t *testing.T) {
	hexaddr := testutil.HexDeploymentAddress(t)
	args := []string{query.QueryCommand().Name(), "order", hexaddr}
	base := baseCommand()
	base.AddCommand(query.QueryCommand())
	base.SetArgs(args)
	err := base.Execute()
	assert.Error(t, err)
}

func TestProviderQuery_NoNode(t *testing.T) {
	hexaddr := testutil.HexDeploymentAddress(t)
	args := []string{query.QueryCommand().Name(), "provider", hexaddr}
	base := baseCommand()
	base.AddCommand(query.QueryCommand())
	base.SetArgs(args)
	err := base.Execute()
	assert.Error(t, err)
}
