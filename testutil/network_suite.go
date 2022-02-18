package testutil

import (
	"fmt"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdknetworktest "github.com/cosmos/cosmos-sdk/testutil/network"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"reflect"
	"time"
)

type NetworkTestSuite struct {
	suite.Suite
	cfg sdknetworktest.Config
	network *sdknetworktest.Network
	testIdx int

	kr keyring.Keyring
}

func NewNetworkTestSuite(cfg *sdknetworktest.Config) NetworkTestSuite {
	nts := NetworkTestSuite{}
	if cfg == nil {
		nts.cfg = sdknetworktest.DefaultConfig()
		//nts.cfg.NumValidators = 1
	} else {
		nts.cfg = *cfg
	}

	return nts
}

func (nts *NetworkTestSuite) countTests() int {
	return reflect.ValueOf(nts).NumMethod()
}

func (nts *NetworkTestSuite) SetupSuite(){
	nts.kr = Keyring(nts.T())
	nts.network = sdknetworktest.New(nts.T(), nts.cfg)

	_, err := nts.network.WaitForHeightWithTimeout(1, time.Second * 30)
	require.NoError(nts.T(), err)

	walletCount := nts.countTests()
	var msgs []sdk.Msg

	for i := 0; i != walletCount ; i++{
		name := fmt.Sprintf("wallet%d", i)
		kinfo, str, err := nts.kr.NewMnemonic(name, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
		require.NoError(nts.T(), err)
		require.NotEmpty(nts.T(), str)

		toAddr := kinfo.GetAddress()

		coins := sdk.NewCoins(sdk.NewCoin(nts.Config().BondDenom, sdk.NewInt(1)))
		msg := banktypes.NewMsgSend(nts.Validator().Address,toAddr, coins)
		msgs = append(msgs, msg)
	}

	txf := tx.NewFactoryCLI(nts.Context(), &pflag.FlagSet{})
	txf = txf.WithSignMode(signing.SignMode_SIGN_MODE_DIRECT)
	txf = txf.WithSimulateAndExecute(false)

	require.Equal(nts.T(), "node0", nts.Context().GetFromName())
	keyInfo, err := txf.Keybase().Key(nts.Context().GetFromName())
	require.NoError(nts.T(), err)
	require.NotNil(nts.T(), keyInfo)


	num, seq , err := txf.AccountRetriever().GetAccountNumberSequence(nts.Context(), nts.Validator().Address)
	require.NoError(nts.T(), err)
	txf = txf.WithAccountNumber(num)
	txf = txf.WithSequence(seq)
	_, adjusted, err := tx.CalculateGas(nts.Context(), txf, msgs...)
	require.NoError(nts.T(), err)
	txf = txf.WithGas(adjusted)

	txb, err := tx.BuildUnsignedTx(txf, msgs...)
	require.NoError(nts.T(), err)

	txb.SetFeeGranter(nts.Context().GetFeeGranterAddress())

	require.NoError(nts.T(), tx.Sign(txf, nts.Context().GetFromName(), txb, true))
	txBytes, err := nts.Context().TxConfig.TxEncoder()(txb.GetTx())

	txr, err := nts.Context().BroadcastTxSync(txBytes)
	require.NoError(nts.T(), err)
	require.NotEqual(nts.T(), txr.Code, 0)
}

func (nts *NetworkTestSuite) Validator(idxT ...int) *sdknetworktest.Validator {
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
	return k.GetAddress()
}

func (nts *NetworkTestSuite) ContextForTest() sdkclient.Context {
	result := nts.Context()
	k, err := nts.kr.Key(nts.WalletNameForTest())
	require.NoError(nts.T(), err)
	return result.WithKeyring(nts.kr).WithFromAddress(k.GetAddress()).WithFrom(nts.WalletNameForTest())
}

func (nts *NetworkTestSuite) Network() *sdknetworktest.Network {
	return nts.network
}

func (nts *NetworkTestSuite) Context(idxT ...int) sdkclient.Context {
	validator := nts.Validator()
	idx := 0
	if len(idxT) != 0 {
		idx = idxT[0]
	}
	result := validator.ClientCtx

	return result.WithFromAddress(validator.Address).WithFromName(fmt.Sprintf("node%d", idx))
}

func (nts *NetworkTestSuite) Config() sdknetworktest.Config {
	return nts.cfg
}

func (nts *NetworkTestSuite) SetupTest(){
	nts.testIdx++
}

