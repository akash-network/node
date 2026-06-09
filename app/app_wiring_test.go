package app

import (
	"bytes"
	"testing"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"pkg.akt.dev/go/sdkutil"
	"pkg.akt.dev/node/v3/x/market"
	"pkg.akt.dev/node/v3/x/provider"
	"pkg.akt.dev/node/v3/x/verification"
)

func TestOrderEndBlockersRunsVerificationBetweenMarketAndGov(t *testing.T) {
	order := orderEndBlockers([]string{
		authtypes.ModuleName,
		govtypes.ModuleName,
		market.ModuleName,
		minttypes.ModuleName,
		stakingtypes.ModuleName,
		verification.ModuleName,
	})

	require.Less(t, moduleIndex(t, order, market.ModuleName), moduleIndex(t, order, verification.ModuleName))
	require.Less(t, moduleIndex(t, order, verification.ModuleName), moduleIndex(t, order, govtypes.ModuleName))
	require.Less(t, moduleIndex(t, order, govtypes.ModuleName), moduleIndex(t, order, stakingtypes.ModuleName))
}

func TestOrderInitGenesisRunsVerificationAfterProvider(t *testing.T) {
	order := orderInitGenesis(nil)

	require.Less(t, moduleIndex(t, order, provider.ModuleName), moduleIndex(t, order, verification.ModuleName))
	require.Less(t, moduleIndex(t, order, verification.ModuleName), moduleIndex(t, order, market.ModuleName))
}

func TestVerificationModuleAccountBlocksDirectBankSend(t *testing.T) {
	application := Setup(WithCheckTx(true))
	ctx := application.NewProposalContext(tmproto.Header{Height: 1})

	perms, exists := ModuleAccountPerms()[verification.ModuleName]
	require.True(t, exists)
	require.Nil(t, perms)

	recipient := authtypes.NewModuleAddress(verification.ModuleName)
	require.True(t, application.ModuleAccountAddrs()[recipient.String()])
	require.True(t, application.BlockedAddrs()[recipient.String()])

	require.NotNil(t, application.Keepers.Cosmos.Acct.GetModuleAccount(ctx, minttypes.ModuleName))
	require.NotNil(t, application.Keepers.Cosmos.Acct.GetModuleAccount(ctx, verification.ModuleName))

	sender := sdk.AccAddress(bytes.Repeat([]byte{1}, 20))
	account := application.Keepers.Cosmos.Acct.NewAccountWithAddress(ctx, sender)
	application.Keepers.Cosmos.Acct.SetAccount(ctx, account)

	coins := sdk.NewCoins(sdk.NewInt64Coin(sdkutil.DenomUakt, 1000))
	application.Keepers.Cosmos.Bank.SetSendEnabled(ctx, sdkutil.DenomUakt, true)
	require.NoError(t, application.Keepers.Cosmos.Bank.MintCoins(ctx, minttypes.ModuleName, coins))
	require.NoError(t, application.Keepers.Cosmos.Bank.SendCoinsFromModuleToAccount(ctx, minttypes.ModuleName, sender, coins))

	_, err := bankkeeper.NewMsgServerImpl(application.Keepers.Cosmos.Bank).Send(ctx, &banktypes.MsgSend{
		FromAddress: sender.String(),
		ToAddress:   recipient.String(),
		Amount:      coins,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not allowed to receive funds")
}

func moduleIndex(t testing.TB, modules []string, module string) int {
	t.Helper()

	for idx, name := range modules {
		if name == module {
			return idx
		}
	}

	t.Fatalf("module %s not found in %v", module, modules)
	return -1
}
