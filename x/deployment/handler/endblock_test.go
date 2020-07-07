package handler_test

import (
	"fmt"
	"testing"

	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/deployment/handler"
	"github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/stretchr/testify/assert"
)

func TestEndBlock(t *testing.T) {
	suite := setupTestSuite(t)
	d0 := testutil.Deployment(suite.t)
	g0 := testutil.DeploymentGroup(suite.t, d0.DeploymentID, uint32(5))
	g1 := testutil.DeploymentGroup(suite.t, d0.DeploymentID, uint32(100))
	g1.State = types.GroupClosed

	d1 := testutil.Deployment(suite.t)
	d1.State = types.DeploymentClosed
	g2 := testutil.DeploymentGroup(suite.t, d1.DeploymentID, uint32(8))

	// create deployments in storage
	df := func(s *testSuite, d types.Deployment, groups ...types.Group) {
		grps := make([]types.GroupSpec, 0, len(groups))
		for _, g := range groups {
			grps = append(grps, g.GroupSpec)
		}
		m := types.MsgCreateDeployment{
			ID:     d.ID(),
			Groups: grps,
		}
		_, err := s.handler(s.ctx, m)
		assert.NoError(s.t, err)

		if d.State == types.DeploymentClosed {
			m := types.MsgCloseDeployment{
				ID: d.ID(),
			}
			_, err := s.handler(s.ctx, m)
			assert.NoError(s.t, err)
		}
	}
	df(suite, d0, g0, g1)
	df(suite, d1, g2)

	// Execute EndBlock method
	handler.OnEndBlock(suite.ctx, suite.dkeeper, suite.mkeeper)

	// Check results of EndBlock
	gx := suite.dkeeper.GetGroups(suite.ctx, d0.ID())
	assert.NotEmpty(t, gx, "no groups returned from keeper")
	for _, g := range gx {
		orderCreated := false
		suite.mkeeper.WithOrdersForGroup(suite.ctx, g.ID(), func(o mtypes.Order) bool {
			suite.t.Logf("Order for group: %#v found", o.GroupID())
			orderCreated = true
			return true
		})
		assert.True(t, orderCreated, "order was not created for a group")
	}

	// d1 is a closed group, assert no orders created
	gy := suite.dkeeper.GetGroups(suite.ctx, d1.ID())
	assert.Len(suite.t, gy, 1, "un-expected number of groups:", len(gy))
	if len(gy) == 1 {
		suite.mkeeper.WithOrdersForGroup(suite.ctx, gy[0].ID(), func(o mtypes.Order) bool {
			suite.t.Error(("deployment state was closed, order should not have been created."))
			return false
		})
	}
}

func TestEndBlockGroups(t *testing.T) {
	suite := setupTestSuite(t)
	df := func(s *testSuite, d types.Deployment, groups ...types.Group) {
		grps := make([]types.GroupSpec, 0, len(groups))
		for _, g := range groups {
			grps = append(grps, g.GroupSpec)
		}
		m := types.MsgCreateDeployment{
			ID:     d.ID(),
			Groups: grps,
		}
		_, err := s.handler(s.ctx, m)
		assert.NoError(s.t, err)
	}
	deployments := make([]types.Deployment, 0)
	for i := 0; i < 10; i++ {
		t.Run(fmt.Sprintf("%d-create-deployment", i), func(t *testing.T) {
			d0 := testutil.Deployment(suite.t)
			d0.State = types.DeploymentActive
			g0 := testutil.DeploymentGroup(suite.t, d0.DeploymentID, uint32(5))
			g1 := testutil.DeploymentGroup(suite.t, d0.DeploymentID, uint32(100))
			deployments = append(deployments, d0)
			df(suite, d0, g0, g1)

			t.Run("assert groups' state are open", func(t *testing.T) {
				groups := suite.dkeeper.GetGroups(suite.ctx, d0.ID())
				assert.Len(t, groups, 2)
				for _, g := range groups {
					assert.Equal(t, g.State, types.GroupOpen)
				}
			})
		})
	}
	openGroups := 0
	suite.dkeeper.WithOpenGroups(suite.ctx, func(g types.Group) bool {
		openGroups++
		return false
	})
	assert.Equal(suite.t, len(deployments)*2, openGroups)

	// Execute EndBlock method
	handler.OnEndBlock(suite.ctx, suite.dkeeper, suite.mkeeper)

	// Assert no Open groups are left
	leftOverGroups := 0
	suite.dkeeper.WithOpenGroups(suite.ctx, func(g types.Group) bool {
		leftOverGroups++
		return false
	})
	assert.Equal(suite.t, 0, leftOverGroups)

	// Use normal methods to assert Group statuses
	for _, dep := range deployments {
		groups := suite.dkeeper.GetGroups(suite.ctx, dep.ID())
		assert.Len(suite.t, groups, 2)
		assert.NotEqual(suite.t, types.GroupOpen, groups[0].State)
		assert.NotEqual(suite.t, types.GroupOpen, groups[1].State)
		assert.Error(suite.t, groups[0].ValidateOrderable())
		assert.Error(suite.t, groups[1].ValidateOrderable())
	}
}
