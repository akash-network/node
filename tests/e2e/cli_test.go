//go:build e2e.integration

package e2e

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/suite"

	"pkg.akt.dev/node/testutil"
)

var DefaultDeposit = sdk.NewCoin("uakt", sdk.NewInt(5000000))

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
