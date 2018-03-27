package main

import (
	"testing"

	"github.com/ovrclk/akash/cmd/akash/query"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/assert"
)

func TestAccountQuery_NoNode(t *testing.T) {
	info, _ := testutil.NewNamedKey(t)
	args := []string{query.QueryCommand().Name(), "account", info.Address.String()}
	base := baseCommand()
	base.AddCommand(query.QueryCommand())
	base.SetArgs(args)
	err := base.Execute()
	assert.Error(t, err)
}

func TestDeploymentQuery_NoNode(t *testing.T) {
	info, _ := testutil.NewNamedKey(t)
	args := []string{query.QueryCommand().Name(), "deployment", info.Address.String()}
	base := baseCommand()
	base.AddCommand(query.QueryCommand())
	base.SetArgs(args)
	err := base.Execute()
	assert.Error(t, err)
}

func TestOrderQuery_NoNode(t *testing.T) {
	info, _ := testutil.NewNamedKey(t)
	args := []string{query.QueryCommand().Name(), "order", info.Address.String()}
	base := baseCommand()
	base.AddCommand(query.QueryCommand())
	base.SetArgs(args)
	err := base.Execute()
	assert.Error(t, err)
}

func TestProviderQuery_NoNode(t *testing.T) {
	info, _ := testutil.NewNamedKey(t)
	args := []string{query.QueryCommand().Name(), "provider", info.Address.String()}
	base := baseCommand()
	base.AddCommand(query.QueryCommand())
	base.SetArgs(args)
	err := base.Execute()
	assert.Error(t, err)
}
