package cli_test

import (
	"fmt"
	"testing"

	"github.com/akash-network/node/testutil"
	"github.com/akash-network/node/x/cert/client/cli"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	certerrors "github.com/akash-network/node/x/cert/errors"
)

const testHost = "foobar.dev"

type certificateCLISuite struct {
	testutil.NetworkTestSuite
}

func (s *certificateCLISuite) TestGeneratePublishAndRevokeServer() {
	result, err := cli.TxGenerateServerExec(s.GoContextForTest(), s.ContextForTest(), s.WalletForTest(), testHost)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	result, err = cli.TxPublishServerExec(s.GoContextForTest(), s.ContextForTest(), s.WalletForTest(),
		fmt.Sprintf("--fees=%d%s", 1000, s.Config().BondDenom),
		"--yes")
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.Network().WaitForNextBlock())
	_ = s.ValidateTx(result.Bytes())

	result, err = cli.TxRevokeServerExec(s.GoContextForTest(), s.ContextForTest(), s.WalletForTest(),
		fmt.Sprintf("--fees=%d%s", 1000, s.Config().BondDenom),
		"--yes")

	require.NoError(s.T(), err)
	require.NoError(s.T(), s.Network().WaitForNextBlock())
	_ = s.ValidateTx(result.Bytes())
}

func (s *certificateCLISuite) TestGenerateServerRequiresArguments() {
	_, err := cli.TxGenerateServerExec(s.GoContextForTest(), s.ContextForTest(), s.WalletForTest(), "")
	require.Error(s.T(), err)
	require.Contains(s.T(), err.Error(), "requires at least 1 arg(s), only received 0")
}

func (s *certificateCLISuite) TestGenerateServerAllowsManyArguments() {
	_, err := cli.TxGenerateServerExec(s.GoContextForTest(), s.ContextForTest(), s.WalletForTest(), "a.dev", "b.dev")
	require.NoError(s.T(), err)
}

func (s *certificateCLISuite) TestGenerateClientRejectsArguments() {
	_, err := cli.TxGenerateClientExec(s.GoContextForTest(), s.ContextForTest(), s.WalletForTest(), testHost)
	require.Error(s.T(), err)
	require.Contains(s.T(), err.Error(), "accepts 0 arg(s), received 1")
}

func (s *certificateCLISuite) TestGeneratePublishAndRevokeClient() {
	result, err := cli.TxGenerateClientExec(s.GoContextForTest(), s.ContextForTest(), s.WalletForTest())
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	result, err = cli.TxPublishClientExec(s.GoContextForTest(), s.ContextForTest(), s.WalletForTest(),
		fmt.Sprintf("--fees=%d%s", 1000, s.Config().BondDenom),
		"--yes")
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.Network().WaitForNextBlock())
	_ = s.ValidateTx(result.Bytes())

	result, err = cli.TxRevokeClientExec(s.GoContextForTest(), s.ContextForTest(), s.WalletForTest(),
		fmt.Sprintf("--fees=%d%s", 1000, s.Config().BondDenom),
		"--yes")

	require.NoError(s.T(), err)
	require.NoError(s.T(), s.Network().WaitForNextBlock())
	_ = s.ValidateTx(result.Bytes())
}

func (s *certificateCLISuite) TestGenerateAndRevokeFailsServer() {
	result, err := cli.TxGenerateServerExec(s.GoContextForTest(), s.ContextForTest(), s.WalletForTest(), testHost)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	_, err = cli.TxRevokeServerExec(s.GoContextForTest(), s.ContextForTest(), s.WalletForTest(),
		fmt.Sprintf("--fees=%d%s", 1000, s.Config().BondDenom),
		"--yes")
	require.ErrorIs(s.T(), err, certerrors.ErrCertificate)
	require.Contains(s.T(), err.Error(), "does not exist on chain")
}

func (s *certificateCLISuite) TestRevokeFailsServer() {
	_, err := cli.TxRevokeServerExec(s.GoContextForTest(), s.ContextForTest(), s.WalletForTest(),
		fmt.Sprintf("--fees=%d%s", 1000, s.Config().BondDenom),
		"--yes",
		"--serial=1")
	require.ErrorIs(s.T(), err, certerrors.ErrCertificate)
	require.Contains(s.T(), err.Error(), "serial 1 does not exist on chain")
}

func (s *certificateCLISuite) TestRevokeFailsClient() {
	_, err := cli.TxRevokeClientExec(s.GoContextForTest(), s.ContextForTest(), s.WalletForTest(),
		fmt.Sprintf("--fees=%d%s", 1000, s.Config().BondDenom),
		"--yes",
		"--serial=1")
	require.ErrorIs(s.T(), err, certerrors.ErrCertificate)
	require.Contains(s.T(), err.Error(), "serial 1 does not exist on chain")
}

func (s *certificateCLISuite) TestGenerateServerNoOverwrite() {
	result, err := cli.TxGenerateServerExec(s.GoContextForTest(), s.ContextForTest(), s.WalletForTest(), testHost)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	_, err = cli.TxGenerateServerExec(s.GoContextForTest(), s.ContextForTest(), s.WalletForTest(), testHost)
	require.ErrorIs(s.T(), err, certerrors.ErrCertificate)
	require.Contains(s.T(), err.Error(), "cannot overwrite")
}

func (s *certificateCLISuite) TestGenerateClientNoOverwrite() {
	result, err := cli.TxGenerateClientExec(s.GoContextForTest(), s.ContextForTest(), s.WalletForTest())
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	_, err = cli.TxGenerateClientExec(s.GoContextForTest(), s.ContextForTest(), s.WalletForTest())
	require.ErrorIs(s.T(), err, certerrors.ErrCertificate)
	require.Contains(s.T(), err.Error(), "cannot overwrite")
}

func TestCertificateCLI(t *testing.T) {
	suite.Run(t, &certificateCLISuite{NetworkTestSuite: testutil.NewNetworkTestSuite(nil, &certificateCLISuite{})})
}
