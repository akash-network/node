package cmd

import (
	"context"
	"fmt"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/testutil/network"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/ovrclk/akash/testutil"
	testutilcli "github.com/ovrclk/akash/testutil/cli"
	"github.com/stretchr/testify/suite"
	"testing"
)

type IntegrationTestSuite struct {
	suite.Suite

	cfg     network.Config
	network *network.Network

	cctx client.Context
}

func (s *IntegrationTestSuite) SetupSuite() {
	s.T().Log("setting up integration test suite")

	cfg := testutil.DefaultConfig()
	cfg.NumValidators = 2
	// cfg.EnableLogging = true

	s.cfg = cfg
	s.network = network.New(s.T(), cfg)

	s.cctx = s.network.Validators[0].ClientCtx
	kr := s.cctx.Keyring
	for i := 0; i != 10; i++ {
		keyName := fmt.Sprintf("testkey-%d", i)
		_, _, err := kr.NewMnemonic(keyName, keyring.English, sdk.FullFundraiserPath, "", hd.Secp256k1)
		s.Require().NoError(err)
	}
}

func (s *IntegrationTestSuite) TearDownSuite() {
	s.T().Log("tearing down integration test suite")
	s.network.Cleanup()
}

func TestIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}

func (s *IntegrationTestSuite) TestExportKeyDoesNotExist() {
	args := []string{"foobar"}
	_, err := testutilcli.ExecTestCLICmd(context.Background(), s.cctx, exportPackedCmd(), args...)
	s.Require().Error(err)
	s.Require().Contains(err.Error(), "no key with name")
}

func (s *IntegrationTestSuite) TestExportOneKey() {
	args := []string{"testkey-3"}
	output, err := testutilcli.ExecTestCLICmd(context.Background(), s.cctx, exportPackedCmd(), args...)
	s.Require().NoError(err)
	s.Require().Greater(len(output.String()), len(PlainTextHeader))
}

func (s *IntegrationTestSuite) TestExportImportPackedKeysRoundTrip() {
	output, err := testutilcli.ExecTestCLICmd(context.Background(), s.cctx, exportPackedCmd())
	s.Require().NoError(err)
	s.Require().Greater(len(output.String()), len(PlainTextHeader))

	// Try and import the keys back in without the overwrite flag
	args := []string{output.String()}
	_, err = testutilcli.ExecTestCLICmd(context.Background(), s.cctx, importPackedCmd(), args...)
	s.Require().Error(err)
	s.Require().ErrorIs(err, errKeyExists)

	// Import into another keyring
	cctxB := s.network.Validators[1].ClientCtx
	krB := cctxB.Keyring
	// Asser the key cannot be found
	_, err = krB.Key("testkey-3")
	s.Require().Error(err)
	s.Require().ErrorIs(err, sdkerrors.ErrKeyNotFound)
	_, err = testutilcli.ExecTestCLICmd(context.Background(), cctxB, importPackedCmd(), args...)
	s.Require().NoError(err)

	// Assert the key can be found
	_, err = krB.Key("testkey-3")
	s.Require().NoError(err)

	// Assert the keys are the same
	krAUnsafe := keyring.NewUnsafe(s.cctx.Keyring)
	krBUnsafe := keyring.NewUnsafe(krB)

	valA, err := krAUnsafe.UnsafeExportPrivKeyHex("testkey-1")
	s.Require().NoError(err)
	valB, err := krBUnsafe.UnsafeExportPrivKeyHex("testkey-1")
	s.Require().NoError(err)
	s.Require().Equal(valA, valB)

	// Try and import the keys back into the original one with the overwrite flag
	args = []string{fmt.Sprintf("--%s", flagOverwrite), output.String()}
	_, err = testutilcli.ExecTestCLICmd(context.Background(), s.cctx, importPackedCmd(), args...)
	s.Require().NoError(err)

}
