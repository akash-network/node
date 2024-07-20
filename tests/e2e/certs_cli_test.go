//go:build e2e.integration

package e2e

import (
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akashd/testutil"

	"pkg.akt.dev/go/cli"
	utiltls "pkg.akt.dev/go/util/tls"
)

const certTestHost = "foobar.dev"

type certificateIntegrationTestSuite struct {
	*testutil.NetworkTestSuite
}

func (s *certificateIntegrationTestSuite) TestGeneratePublishAndRevokeServer() {
	result, err := cli.TxGenerateServerExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		certTestHost,
		cli.TestFlags().
			WithFrom(s.WalletForTest()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	result, err = cli.TxPublishServerExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithFrom(s.WalletForTest()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.Network().WaitForNextBlock())
	_ = s.ValidateTx(result.Bytes())

	result, err = cli.TxRevokeServerExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithFrom(s.WalletForTest()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)

	require.NoError(s.T(), err)
	require.NoError(s.T(), s.Network().WaitForNextBlock())
	_ = s.ValidateTx(result.Bytes())
}

func (s *certificateIntegrationTestSuite) TestGenerateServerRequiresArguments() {
	_, err := cli.TxGenerateServerExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		"",
		cli.TestFlags().
			WithFrom(s.WalletForTest()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.Error(s.T(), err)
	require.Contains(s.T(), err.Error(), "requires at least 1 arg(s), only received 0")
}

func (s *certificateIntegrationTestSuite) TestGenerateServerAllowsManyArguments() {
	_, err := cli.TxGenerateServerExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		"a.dev",
		cli.TestFlags().
			With("b.dev").
			WithFrom(s.WalletForTest()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.NoError(s.T(), err)
}

func (s *certificateIntegrationTestSuite) TestGenerateClientRejectsArguments() {
	_, err := cli.TxGenerateClientExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			With("empty").
			WithFrom(s.WalletForTest()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.Error(s.T(), err)
	require.Contains(s.T(), err.Error(), "accepts 0 arg(s), received 1")
}

func (s *certificateIntegrationTestSuite) TestGeneratePublishAndRevokeClient() {
	result, err := cli.TxGenerateClientExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithFrom(s.WalletForTest()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	result, err = cli.TxPublishClientExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithFrom(s.WalletForTest()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.Network().WaitForNextBlock())
	_ = s.ValidateTx(result.Bytes())

	result, err = cli.TxRevokeClientExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithFrom(s.WalletForTest()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)

	require.NoError(s.T(), err)
	require.NoError(s.T(), s.Network().WaitForNextBlock())
	_ = s.ValidateTx(result.Bytes())
}

func (s *certificateIntegrationTestSuite) TestGenerateAndRevokeFailsServer() {
	result, err := cli.TxGenerateServerExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		certTestHost,
		cli.TestFlags().
			WithFrom(s.WalletForTest()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	_, err = cli.TxRevokeServerExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithFrom(s.WalletForTest()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.ErrorIs(s.T(), err, utiltls.ErrCertificate)
	require.Contains(s.T(), err.Error(), "does not exist on chain")
}

func (s *certificateIntegrationTestSuite) TestRevokeFailsServer() {
	_, err := cli.TxRevokeServerExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithFrom(s.WalletForTest()).
			WithSerial("1").
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.ErrorIs(s.T(), err, utiltls.ErrCertificate)
	require.Contains(s.T(), err.Error(), "serial 1 does not exist on chain")
}

func (s *certificateIntegrationTestSuite) TestRevokeFailsClient() {
	_, err := cli.TxRevokeClientExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithFrom(s.WalletForTest()).
			WithSerial("1").
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.ErrorIs(s.T(), err, utiltls.ErrCertificate)
	require.Contains(s.T(), err.Error(), "serial 1 does not exist on chain")
}

func (s *certificateIntegrationTestSuite) TestGenerateServerNoOverwrite() {
	result, err := cli.TxGenerateServerExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		certTestHost,
		cli.TestFlags().
			WithFrom(s.WalletForTest()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	_, err = cli.TxGenerateServerExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		certTestHost,
		cli.TestFlags().
			WithFrom(s.WalletForTest()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.ErrorIs(s.T(), err, utiltls.ErrCertificate)
	require.Contains(s.T(), err.Error(), "cannot overwrite")
}

func (s *certificateIntegrationTestSuite) TestGenerateClientNoOverwrite() {
	result, err := cli.TxGenerateClientExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithFrom(s.WalletForTest()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	_, err = cli.TxGenerateClientExec(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithFrom(s.WalletForTest()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	require.ErrorIs(s.T(), err, utiltls.ErrCertificate)
	require.Contains(s.T(), err.Error(), "cannot overwrite")
}
