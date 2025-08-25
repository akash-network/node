package keeper

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"pkg.akt.dev/go/testutil"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/assert"

	"pkg.akt.dev/go/node/escrow/v1"
)

const denom = "uakt"

func TestSettleFullBlocks(t *testing.T) {
	for _, tt := range []struct {
		name string
		cfg  distTestConfig
	}{
		{
			name: "plenty left",
			cfg: distTestConfig{
				blocks:       5,
				balanceStart: 100,
				rates:        []int64{1, 2},
				balanceEnd:   85,
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(5), sdkmath.LegacyNewDec(10)},
				overdraft:    0,
			},
		},
		{
			name: "use it all",
			cfg: distTestConfig{
				blocks:       5,
				balanceStart: 100,
				rates:        []int64{10, 10},
				balanceEnd:   0,
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(50), sdkmath.LegacyNewDec(50)},
				overdraft:    0,
			},
		},
		{
			name: "ruh roh",
			cfg: distTestConfig{
				blocks:       6,
				balanceStart: 100,
				rates:        []int64{10, 10},
				balanceEnd:   0,
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(60), sdkmath.LegacyNewDec(40)},
				overdraft:    20,
			},
		},
		{
			name: "plenty funds",
			cfg: distTestConfig{
				blocks:       5,
				balanceStart: 100,
				rates:        []int64{1, 2},
				balanceEnd:   85,
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(5), sdkmath.LegacyNewDec(10)},
				overdraft:    0,
			},
		},
		{
			name: "use all funds",
			cfg: distTestConfig{
				blocks:       5,
				balanceStart: 200,
				rates:        []int64{10, 10},
				balanceEnd:   100,
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(50), sdkmath.LegacyNewDec(50)},
				overdraft:    0,
			},
		},
		{
			name: "use all funds with some balance",
			cfg: distTestConfig{
				blocks:       6,
				balanceStart: 200,
				rates:        []int64{10, 10},
				balanceEnd:   80,
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(60), sdkmath.LegacyNewDec(60)},
				overdraft:    0,
			},
		},
		{
			name: "use all funds and balance",
			cfg: distTestConfig{
				blocks:       10,
				balanceStart: 200,
				rates:        []int64{10, 10},
				balanceEnd:   0,
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(100), sdkmath.LegacyNewDec(100)},
				overdraft:    0,
			},
		},
		{
			name: "overdrawn with all funds and balance used up",
			cfg: distTestConfig{
				blocks:       11,
				balanceStart: 200,
				rates:        []int64{10, 10},
				balanceEnd:   0,
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(110), sdkmath.LegacyNewDec(90)},
				overdraft:    20,
			},
		},
	} {
		account, payments, blocks := setupDistTest(t, tt.cfg)

		overdrawn := accountSettleFullBlocks(&account, payments, blocks)
		assert.Equal(t, tt.cfg.overdraft != 0, overdrawn, tt.name)

		assertCoinsEqual(t, sdk.NewInt64DecCoin(denom, tt.cfg.balanceEnd), account.Funds[0].Balance, tt.name)

		for idx := range payments {
			assert.Equal(t, sdk.NewDecCoinFromDec(denom, tt.cfg.transferred[idx]), payments[idx].Balance, tt.name)
		}

		assertCoinsEqual(t, sdk.NewInt64DecCoin(denom, tt.cfg.overdraft), sdk.NewDecCoinFromDec(denom, account.Funds[0].Overdraft), tt.name)
	}
}

type distTestConfig struct {
	blocks       int64
	balanceStart int64
	rates        []int64
	balanceEnd   int64
	transferred  []sdkmath.LegacyDec
	overdraft    int64
}

func setupDistTest(t *testing.T, cfg distTestConfig) (account, []payment, sdkmath.Int) {
	balance := sdk.NewInt64Coin(denom, cfg.balanceStart)
	account := account{
		Account: v1.Account{
			Funds: []v1.Funds{
				{
					Balance:   sdk.NewDecCoinFromCoin(balance),
					Overdraft: sdkmath.LegacyZeroDec(),
				},
			},
			Transferred: sdk.DecCoins{
				sdk.NewInt64DecCoin(denom, 0),
			},
			Deposits: []v1.Deposit{
				{
					Depositor: testutil.AccAddress(t).String(),
					Height:    0,
					Amount:    sdk.NewCoin(balance.Denom, balance.Amount),
					Balance:   sdk.NewDecCoinFromCoin(balance),
				},
			},
		},
	}

	payments := make([]payment, 0, len(cfg.rates))

	blockRate := int64(0)

	for _, rate := range cfg.rates {
		blockRate += rate
		payments = append(payments, payment{
			FractionalPayment: v1.FractionalPayment{
				Rate:    sdk.NewInt64DecCoin(denom, rate),
				Balance: sdk.NewInt64DecCoin(denom, 0),
			},
		})
	}

	return account, payments, sdkmath.NewInt(cfg.blocks)
}

func assertCoinsEqual(t testing.TB, c1 sdk.DecCoin, c2 sdk.DecCoin, msg string) {
	t.Helper()
	if c1.IsZero() {
		if !c2.IsZero() {
			assert.Failf(t, msg, "%v is not zero", c2)
		}
		return
	}
	assert.Equal(t, c1.Amount, c2.Amount, msg)
}
