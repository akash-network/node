//go:build e2e.integration

package e2e

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"pkg.akt.dev/node/testutil"
)

func TestIntegrationGRPC(t *testing.T) {
	dg := &deploymentGRPCRestTestSuite{}
	dg.NetworkTestSuite = testutil.NewNetworkTestSuite(nil, dg)

	cg := &certsGRPCRestTestSuite{}
	cg.NetworkTestSuite = testutil.NewNetworkTestSuite(nil, cg)

	mg := &marketGRPCRestTestSuite{}
	mg.NetworkTestSuite = testutil.NewNetworkTestSuite(nil, mg)

	pg := &providerGRPCRestTestSuite{}
	pg.NetworkTestSuite = testutil.NewNetworkTestSuite(nil, pg)

	suite.Run(t, dg)
	suite.Run(t, cg)
	suite.Run(t, mg)
	suite.Run(t, pg)
}
