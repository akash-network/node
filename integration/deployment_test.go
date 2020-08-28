// +build integration,!mainnet

package integration

import (
	"fmt"
	"testing"

	"github.com/cosmos/cosmos-sdk/tests"
	sdk "github.com/cosmos/cosmos-sdk/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
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

	fooAcc := f.QueryAccount(fooAddr)
	startTokens := sdk.TokensFromConsensusPower(denomStartValue)
	require.Equal(t, startTokens, fooAcc.GetCoins().AmountOf(denom))

	// Create deployment
	f.TxCreateDeployment(deploymentFilePath, fmt.Sprintf("--from=%s", keyFoo), "-y")
	tests.WaitForNextNBlocksTM(1, f.Port)

	// test query deployments
	deployments, err := f.QueryDeployments()
	require.NoError(t, err)
	require.Len(t, deployments, 1, "Deployment Create Failed")
	require.Equal(t, fooAddr.String(), deployments[0].Deployment.DeploymentID.Owner.String())

	// test query deployment
	createdDep := deployments[0]
	deployment := f.QueryDeployment(createdDep.Deployment.DeploymentID)
	require.Equal(t, createdDep, deployment)

	// test query deployments with owner filter
	deployments, err = f.QueryDeployments(fmt.Sprintf("--owner=%s", fooAddr.String()))
	require.NoError(t, err, "Error when fetching deployments with owner filter")
	require.Len(t, deployments, 1)

	// test updating deployment
	execSuccess, stdOut, stdErr := f.TxUpdateDeployment(fmt.Sprintf("--from=%s --dseq=%d",
		keyFoo, deployment.DeploymentID.DSeq), "-y")
	require.True(t, execSuccess)
	require.Empty(t, stdErr)
	require.NotEmpty(t, stdOut)

	deploymentV2 := f.QueryDeployment(createdDep.Deployment.DeploymentID)
	require.NotEqual(t, deployment.Version, deploymentV2.Version)

	// test query deployments with wrong owner value
	deployments, err = f.QueryDeployments("--owner=cosmos102ruvpv2srmunfffxavttxnhezln6fnc3pf7tt")
	require.Error(t, err)

	// test query deployments with filters
	deployments, err = f.QueryDeployments("--state=closed")
	require.NoError(t, err)
	require.Len(t, deployments, 0)

	// test query deployments with wrong state filter
	deployments, err = f.QueryDeployments("--state=hello")
	require.Error(t, err)

	// Close deployment
	f.TxCloseDeployment(fmt.Sprintf("--from=%s --dseq=%v", keyFoo, createdDep.Deployment.DeploymentID.DSeq), "-y")
	tests.WaitForNextNBlocksTM(1, f.Port)

	// test query deployments
	deployments, err = f.QueryDeployments()
	require.NoError(t, err)
	require.Len(t, deployments, 1)
	require.Equal(t, dtypes.DeploymentClosed, deployments[0].Deployment.State, "Deployment Close Failed")

	// test query deployments with state filter closed
	deployments, err = f.QueryDeployments("--state=closed")
	require.NoError(t, err)
	require.Len(t, deployments, 1)

	f.Cleanup()
}
