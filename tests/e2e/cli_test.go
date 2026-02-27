//go:build e2e.integration

package e2e

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/suite"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/testutil"
)

var DefaultDeposit = sdk.NewCoin(sdkutil.DenomUact, sdkmath.NewInt(5000000))

func TestIntegrationCLI(t *testing.T) {
	di := &deploymentIntegrationTestSuite{}
	di.NetworkTestSuite = testutil.NewNetworkTestSuite(nil, di)

	ci := &certificateIntegrationTestSuite{}
	ci.NetworkTestSuite = testutil.NewNetworkTestSuite(nil, ci)

	mi := &marketIntegrationTestSuite{}
	mi.NetworkTestSuite = testutil.NewNetworkTestSuite(nil, mi)

	pi := &providerIntegrationTestSuite{}
	pi.NetworkTestSuite = testutil.NewNetworkTestSuite(nil, pi)

	oi := &oracleIntegrationTestSuite{}
	oi.NetworkTestSuite = testutil.NewNetworkTestSuite(nil, oi)

	bi := &bmeIntegrationTestSuite{}
	bi.NetworkTestSuite = testutil.NewNetworkTestSuite(nil, bi)

	suite.Run(t, di)
	suite.Run(t, ci)
	suite.Run(t, mi)
	suite.Run(t, pi)
	suite.Run(t, oi)
	suite.Run(t, bi)
}
