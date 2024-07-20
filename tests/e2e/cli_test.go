//go:build e2e.integration

package e2e

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"pkg.akt.dev/akashd/testutil"
)

func TestIntegrationCLI(t *testing.T) {
	di := &deploymentIntegrationTestSuite{}
	di.NetworkTestSuite = testutil.NewNetworkTestSuite(nil, di)

	ci := &certificateIntegrationTestSuite{}
	ci.NetworkTestSuite = testutil.NewNetworkTestSuite(nil, ci)

	mi := &marketIntegrationTestSuite{}
	mi.NetworkTestSuite = testutil.NewNetworkTestSuite(nil, mi)

	pi := &providerIntegrationTestSuite{}
	pi.NetworkTestSuite = testutil.NewNetworkTestSuite(nil, pi)

	suite.Run(t, di)
	suite.Run(t, ci)
	suite.Run(t, mi)
	suite.Run(t, pi)
}
