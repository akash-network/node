package cli_test

import (
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/cert/client/cli"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"testing"
)

const testHost = "foobar.dev"

type certificateCLISuite struct {
	testutil.NetworkTestSuite
}

func (s *certificateCLISuite) TestGenerateAndPublishServer(){
	result, err := cli.TxGenerateServerExec(s.ContextForTest(), s.WalletForTest(), testHost)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	result, err = cli.TxPublishServerExec(s.ContextForTest(), s.WalletForTest(), testHost)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)
}

func (s *certificateCLISuite) TestGenerateAndPublishClient(){
	result, err := cli.TxGenerateClientExec(s.ContextForTest(), s.WalletForTest())
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	result, err = cli.TxPublishClientExec(s.ContextForTest(), s.WalletForTest())
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)
}


func (s *certificateCLISuite) TestGenerateServerNoOverwrite(){
	result, err := cli.TxGenerateServerExec(s.ContextForTest(), s.WalletForTest(), testHost)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	result, err = cli.TxGenerateServerExec(s.ContextForTest(), s.WalletForTest(), testHost)
	require.ErrorIs(s.T(), err, cli.ErrCertificate)
	require.Contains(s.T(), err.Error(), "cannot overwrite")

}

func (s *certificateCLISuite) TestGenerateClientNoOverwrite(){
	result, err := cli.TxGenerateClientExec(s.ContextForTest(), s.WalletForTest())
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	result, err = cli.TxGenerateClientExec(s.ContextForTest(), s.WalletForTest())
	require.ErrorIs(s.T(), err, cli.ErrCertificate)
	require.Contains(s.T(), err.Error(), "cannot overwrite")

}


func TestCertificateCLI(t *testing.T){
	suite.Run(t, &certificateCLISuite{NetworkTestSuite:testutil.NewNetworkTestSuite(nil)})
}
