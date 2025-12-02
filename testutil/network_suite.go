package testutil

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	sdkmath "cosmossdk.io/math"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	cosmosauthtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	"github.com/cosmos/gogoproto/jsonpb"

	"pkg.akt.dev/go/cli"
	cflags "pkg.akt.dev/go/cli/flags"
	clitestutil "pkg.akt.dev/go/cli/testutil"
	arpcclient "pkg.akt.dev/go/node/client"
	"pkg.akt.dev/go/sdkutil"
	sdktestutil "pkg.akt.dev/go/testutil"

	"pkg.akt.dev/node/testutil/network"
)

type NetworkTestSuite struct {
	*suite.Suite
	cfg           network.Config
	network       *network.Network
	testIdx       int
	kr            keyring.Keyring
	testCtx       context.Context
	cancelTestCtx context.CancelFunc
	container     interface{}
	cliCtx        context.Context   // Context with address codec for CLI commands
	cliCctx       sdkclient.Context // Client context with proper Akash RPC client
}

func NewNetworkTestSuite(cfg *network.Config, container interface{}) *NetworkTestSuite {
	nts := &NetworkTestSuite{
		Suite:     &suite.Suite{},
		testIdx:   -1,
		container: container,
	}
	if cfg == nil {
		nts.cfg = network.DefaultConfig(NewTestNetworkFixture)
		nts.cfg.NumValidators = 1
	} else {
		nts.cfg = *cfg
	}

	return nts
}

func (nts *NetworkTestSuite) countTests() int {
	vof := reflect.TypeOf(nts.container)

	cnt := 0
	for i := 0; i != vof.NumMethod(); i++ {
		method := vof.Method(i)
		methodName := method.Name
		if strings.HasPrefix(methodName, "Test") {
			cnt++
		}
	}

	return cnt
}

func (nts *NetworkTestSuite) TearDownSuite() {
	nts.network.Cleanup()
}

func (nts *NetworkTestSuite) SetupSuite() {
	nts.kr = sdktestutil.NewTestKeyring(nts.cfg.Codec)
	nts.network = network.New(nts.T(), nts.cfg)

	_, err := nts.network.WaitForHeightWithTimeout(1, time.Second*30)
	require.NoError(nts.T(), err)

	walletCount := nts.countTests()
	nts.T().Logf("setting up %d wallets for test", walletCount)

	// Set up context with address codec (required by CLI commands)
	signingOpts := sdkutil.NewSigningOptions()
	nts.cliCtx = context.WithValue(context.Background(), cli.ContextTypeAddressCodec, signingOpts.AddressCodec)
	nts.cliCtx = context.WithValue(nts.cliCtx, cli.ContextTypeValidatorCodec, signingOpts.ValidatorAddressCodec)

	val := nts.Validator()

	// Create proper Akash RPC client that implements the RPCClient interface
	client, err := arpcclient.NewClient(nts.cliCtx, val.RPCAddress)
	require.NoError(nts.T(), err)

	nts.cliCctx = val.ClientCtx.WithClient(client)

	for i := 0; i != walletCount; i++ {
		name := fmt.Sprintf("wallet%d", i)
		kinfo, str, err := nts.kr.NewMnemonic(name, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
		require.NoError(nts.T(), err)
		require.NotEmpty(nts.T(), str)

		toAddr, err := kinfo.GetAddress()
		require.NoError(nts.T(), err)

		// Fund with enough for deposits (5M uakt each) plus gas fees
		coins := sdk.NewCoins(sdk.NewCoin(nts.Config().BondDenom, sdkmath.NewInt(50000000)))

		_, err = clitestutil.ExecSend(
			nts.cliCtx,
			nts.cliCctx,
			cli.TestFlags().
				With(
					val.Address.String(),
					toAddr.String(),
					coins.String()).
				WithFrom(val.Address.String()).
				WithGasAuto().
				WithSkipConfirm().
				WithBroadcastModeBlock()...,
		)
		require.NoError(nts.T(), err)
		require.NoError(nts.T(), nts.network.WaitForNextBlock())
	}
}

// CLIContext returns the context configured with address codec for CLI commands
func (nts *NetworkTestSuite) CLIContext() context.Context {
	return nts.cliCtx
}

// CLIClientContext returns the client context with proper Akash RPC client
func (nts *NetworkTestSuite) CLIClientContext() sdkclient.Context {
	return nts.cliCctx
}

func (nts *NetworkTestSuite) Validator(idxT ...int) *network.Validator {
	idx := 0
	if len(idxT) != 0 {
		if len(idxT) > 1 {
			nts.T().Fatal("pass zero or one arguments to Validator()")
		}
		idx = idxT[0]

		if idx > len(nts.network.Validators) {
			nts.T().Fatal("not enough validators for each test")
		}
	}
	return nts.network.Validators[idx]
}

func (nts *NetworkTestSuite) WalletNameForTest() string {
	return fmt.Sprintf("wallet%d", nts.testIdx)
}

func (nts *NetworkTestSuite) WalletForTest() sdk.AccAddress {
	k, err := nts.kr.Key(nts.WalletNameForTest())
	require.NoError(nts.T(), err)
	addr, err := k.GetAddress()
	require.NoError(nts.T(), err)

	return addr
}

func (nts *NetworkTestSuite) ClientContextForTest() sdkclient.Context {
	cctx := nts.ClientContext()
	k, err := nts.kr.Key(nts.WalletNameForTest())
	require.NoError(nts.T(), err)

	addr, err := k.GetAddress()
	require.NoError(nts.T(), err)

	cctx = cctx.WithKeyring(nts.kr).
		WithFromAddress(addr).
		WithFromName(nts.WalletNameForTest()).
		WithBroadcastMode(cflags.BroadcastBlock).
		WithSignModeStr("direct")

	return cctx
}

func (nts *NetworkTestSuite) ContextForTest() context.Context {
	return nts.testCtx
}

func (nts *NetworkTestSuite) Network() *network.Network {
	return nts.network
}

func (nts *NetworkTestSuite) ClientContext(idxT ...int) sdkclient.Context {
	validator := nts.Validator()
	idx := 0
	if len(idxT) != 0 {
		idx = idxT[0]
	}
	// Use the properly configured client context with Akash RPC client
	result := nts.cliCctx

	return result.WithFromAddress(validator.Address).WithFromName(fmt.Sprintf("node%d", idx))
}

func (nts *NetworkTestSuite) Config() network.Config {
	return nts.cfg
}

func (nts *NetworkTestSuite) SetupTest() {
	nts.testIdx++
	nts.testCtx, nts.cancelTestCtx = context.WithTimeout(context.Background(), 30*time.Second)
}

func (nts *NetworkTestSuite) TearDownTest() {
	nts.cancelTestCtx()
}

func (nts *NetworkTestSuite) ValidateTx(resultData []byte) string {
	nts.T().Helper()

	var resp sdk.TxResponse

	err := jsonpb.Unmarshal(bytes.NewBuffer(resultData), &resp)
	require.NoError(nts.T(), err, "failed trying to unmarshal JSON transaction result")

	for {
		res, err := cosmosauthtx.QueryTx(nts.ClientContextForTest(), resp.TxHash)
		if err != nil {
			ctxDone := nts.ContextForTest().Err() != nil
			if ctxDone {
				require.NoErrorf(nts.T(), err, "failed querying for transaction %q", resp.TxHash)
			} else {
				nts.T().Logf("waiting before checking for TX %s", resp.TxHash)
				select {
				case <-nts.ContextForTest().Done():
					require.NoError(nts.T(), nts.ContextForTest().Err())
				case <-time.After(500 * time.Millisecond):

				}
			}
			continue
		}

		require.Zero(nts.T(), res.Code, res, "expected response code in transaction to be zero")
		break
	}

	return resp.TxHash
}
