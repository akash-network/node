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
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/gogoproto/jsonpb"

	cflags "pkg.akt.dev/go/cli/flags"
	aclient "pkg.akt.dev/go/node/client/discovery"
	cltypes "pkg.akt.dev/go/node/client/types"
	cclient "pkg.akt.dev/go/node/client/v1beta3"
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
	var msgs []sdk.Msg
	//
	for i := 0; i != walletCount; i++ {
		name := fmt.Sprintf("wallet%d", i)
		kinfo, str, err := nts.kr.NewMnemonic(name, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
		require.NoError(nts.T(), err)
		require.NotEmpty(nts.T(), str)

		toAddr, err := kinfo.GetAddress()
		require.NoError(nts.T(), err)

		coins := sdk.NewCoins(sdk.NewCoin(nts.Config().BondDenom, sdkmath.NewInt(1000000)))
		msg := banktypes.NewMsgSend(nts.Validator().Address, toAddr, coins)
		msgs = append(msgs, msg)
	}

	ctx := context.Background()
	cctx := nts.ClientContext().WithFrom(nts.network.Validators[0].Address.String())

	cl, err := aclient.DiscoverClient(
		ctx,
		cctx,
		cltypes.WithGas(cltypes.GasSetting{Simulate: true}),
		cltypes.WithGasAdjustment(1.5),
		cltypes.WithGasPrices("0.0025uakt"),
	)
	require.NoError(nts.T(), err)

	_, err = cl.Tx().BroadcastMsgs(ctx, msgs, cclient.WithBroadcastMode("block"))
	require.NoError(nts.T(), err)

	// txf, err := tx.NewFactoryCLI(nts.ClientContext(), &pflag.FlagSet{})
	// require.NoError(nts.T(), err)
	//
	// txf = txf.WithSignMode(signing.SignMode_SIGN_MODE_DIRECT)
	// txf = txf.WithSimulateAndExecute(false)
	//
	// require.Equal(nts.T(), "node0", nts.ClientContext().GetFromName())
	// keyInfo, err := txf.Keybase().Key(nts.ClientContext().GetFromName())
	// require.NoError(nts.T(), err)
	// require.NotNil(nts.T(), keyInfo)
	//
	// num, seq, err := txf.AccountRetriever().GetAccountNumberSequence(nts.ClientContext(), nts.Validator().Address)
	// require.NoError(nts.T(), err)
	// txf = txf.WithAccountNumber(num)
	// txf = txf.WithSequence(seq)
	// // txf = txf.WithGas(uint64(150000 * nts.countTests()))                 // Just made this up
	// txf = txf.WithFees(fmt.Sprintf("%d%s", 100, nts.Config().BondDenom)) // Just made this up
	//
	// txb, err := txf.BuildUnsignedTx(msgs...)
	// require.NoError(nts.T(), err)
	//
	// txb.SetFeeGranter(nts.ClientContext().GetFeeGranterAddress())
	//
	// require.NoError(nts.T(), tx.Sign(txf, nts.ClientContext().GetFromName(), txb, true))
	// txBytes, err := nts.ClientContext().TxConfig.TxEncoder()(txb.GetTx())
	// require.NoError(nts.T(), err)
	//
	// txr, err := nts.ClientContext().BroadcastTxSync(txBytes)
	// require.NoError(nts.T(), err)
	// require.Equal(nts.T(), uint32(0), txr.Code)

	// lctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	// defer cancel()
	//
	// for lctx.Err() == nil {
	// 	// check the TX
	// 	txStatus, err := cl.Query()..QueryTx(nts.Context(), txr.TxHash)
	// 	if err != nil {
	// 		if strings.Contains(err.Error(), ") not found") {
	// 			continue
	// 		}
	// 	}
	// 	require.NoError(nts.T(), err)
	// 	require.NotNil(nts.T(), txStatus)
	// 	require.Equalf(nts.T(), uint32(0), txStatus.Code, "tx status is %v", txStatus)
	// 	break
	// }
	// require.NoError(nts.T(), lctx.Err())

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
	result := validator.ClientCtx

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
