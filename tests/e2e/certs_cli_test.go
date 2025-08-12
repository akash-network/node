//go:build e2e.integration

package e2e

import (
	"github.com/stretchr/testify/require"
	clitestutil "pkg.akt.dev/go/cli/testutil"

	"pkg.akt.dev/node/testutil"

	"pkg.akt.dev/go/cli"
	utiltls "pkg.akt.dev/go/util/tls"
)

const certTestHost = "foobar.dev"

type certificateIntegrationTestSuite struct {
	*testutil.NetworkTestSuite
}

func (s *certificateIntegrationTestSuite) TestGeneratePublishAndRevokeServer() {
	result, err := clitestutil.TxGenerateServerExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			With(certTestHost).
			WithFrom(s.WalletForTest().String()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	result, err = clitestutil.TxPublishServerExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithFrom(s.WalletForTest().String()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.Network().WaitForNextBlock())
	_ = s.ValidateTx(result.Bytes())

	result, err = clitestutil.TxRevokeServerExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithFrom(s.WalletForTest().String()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)

	require.NoError(s.T(), err)
	require.NoError(s.T(), s.Network().WaitForNextBlock())
	_ = s.ValidateTx(result.Bytes())
}

func (s *certificateIntegrationTestSuite) TestGenerateServerRequiresArguments() {
	_, err := clitestutil.TxGenerateServerExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			With("").
			WithFrom(s.WalletForTest().String()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.Error(s.T(), err)
	require.Contains(s.T(), err.Error(), "requires at least 1 arg(s), only received 0")
}

func (s *certificateIntegrationTestSuite) TestGenerateServerAllowsManyArguments() {
	_, err := clitestutil.TxGenerateServerExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			With("a.dev", "b.dev").
			WithFrom(s.WalletForTest().String()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.NoError(s.T(), err)
}

func (s *certificateIntegrationTestSuite) TestGenerateClientRejectsArguments() {
	_, err := clitestutil.TxGenerateClientExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			With("empty").
			WithFrom(s.WalletForTest().String()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.Error(s.T(), err)
	require.Contains(s.T(), err.Error(), "accepts 0 arg(s), received 1")
}

func (s *certificateIntegrationTestSuite) TestGeneratePublishAndRevokeClient() {
	result, err := clitestutil.TxGenerateClientExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithFrom(s.WalletForTest().String()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	result, err = clitestutil.TxPublishClientExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithFrom(s.WalletForTest().String()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.Network().WaitForNextBlock())
	_ = s.ValidateTx(result.Bytes())

	result, err = clitestutil.TxRevokeClientExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithFrom(s.WalletForTest().String()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)

	require.NoError(s.T(), err)
	require.NoError(s.T(), s.Network().WaitForNextBlock())
	_ = s.ValidateTx(result.Bytes())
}

func (s *certificateIntegrationTestSuite) TestGenerateAndRevokeFailsServer() {
	result, err := clitestutil.TxGenerateServerExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			With(certTestHost).
			WithFrom(s.WalletForTest().String()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	_, err = clitestutil.TxRevokeServerExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithFrom(s.WalletForTest().String()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.ErrorIs(s.T(), err, utiltls.ErrCertificate)
	require.Contains(s.T(), err.Error(), "does not exist on chain")
}

func (s *certificateIntegrationTestSuite) TestRevokeFailsServer() {
	_, err := clitestutil.TxRevokeServerExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithFrom(s.WalletForTest().String()).
			WithSerial("1").
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.ErrorIs(s.T(), err, utiltls.ErrCertificate)
	require.Contains(s.T(), err.Error(), "serial 1 does not exist on chain")
}

func (s *certificateIntegrationTestSuite) TestRevokeFailsClient() {
	_, err := clitestutil.TxRevokeClientExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithFrom(s.WalletForTest().String()).
			WithSerial("1").
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.ErrorIs(s.T(), err, utiltls.ErrCertificate)
	require.Contains(s.T(), err.Error(), "serial 1 does not exist on chain")
}

func (s *certificateIntegrationTestSuite) TestGenerateServerNoOverwrite() {
	result, err := clitestutil.TxGenerateServerExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			With(certTestHost).
			WithFrom(s.WalletForTest().String()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	_, err = clitestutil.TxGenerateServerExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			With(certTestHost).
			WithFrom(s.WalletForTest().String()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.ErrorIs(s.T(), err, utiltls.ErrCertificate)
	require.Contains(s.T(), err.Error(), "cannot overwrite")
}

func (s *certificateIntegrationTestSuite) TestGenerateClientNoOverwrite() {
	result, err := clitestutil.TxGenerateClientExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithFrom(s.WalletForTest().String()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	_, err = clitestutil.TxGenerateClientExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithFrom(s.WalletForTest().String()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.ErrorIs(s.T(), err, utiltls.ErrCertificate)
	require.Contains(s.T(), err.Error(), "cannot overwrite")
}
