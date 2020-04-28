// +build integration

package integration

import (
	"fmt"
	"testing"

	"github.com/cosmos/cosmos-sdk/tests"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestDeployment(t *testing.T) {
	t.Parallel()
	f := InitFixtures(t)

	// start akashd server
	proc := f.AkashdStart()

	defer func() {
		_ = proc.Stop(false)
	}()

	// Save key addresses for later use
	fooAddr := f.KeyAddress(keyFoo)

	// fooAcc := f.QueryAccount(fooAddr)
	startTokens := sdk.TokensFromConsensusPower(denomStartValue)
	require.Equal(t, startTokens, f.QueryBalances(fooAddr).AmountOf(denom))

	// Create deployment
	f.TxCreateDeployment(fmt.Sprintf("--from=%s", keyFoo), "-y")
	tests.WaitForNextNBlocksTM(1, f.Port)

	// test query deployments
	deployments := f.QueryDeployments()
	require.Len(t, deployments, 1, "Deployment Create Failed")
	require.Equal(t, fooAddr.String(), deployments[0].Deployment.DeploymentID.Owner.String())

	// test query deployment
	createdDep := deployments[0]
	deployment := f.QueryDeployment(createdDep.Deployment.DeploymentID)
	require.Equal(t, createdDep, deployment)

	// test query deployments with filters
	deployments = f.QueryDeployments("--state=closed")
	require.Len(t, deployments, 0)

	// Close deployment
	f.TxCloseDeployment(fmt.Sprintf("--from=%s --dseq=%v", keyFoo, createdDep.Deployment.DeploymentID.DSeq), "-y")
	tests.WaitForNextNBlocksTM(1, f.Port)

	// test query deployments
	deployments = f.QueryDeployments()
	require.Len(t, deployments, 1)
	require.Equal(t, uint8(1), uint8(deployments[0].Deployment.State), "Deployment Close Failed")

	// test query deployments with state filter closed
	deployments = f.QueryDeployments("--state=closed")
	require.Len(t, deployments, 1)

	f.Cleanup()
}
