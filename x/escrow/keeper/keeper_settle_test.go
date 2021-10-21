package keeper

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	types "github.com/ovrclk/akash/x/escrow/types/v1beta2"
	"github.com/stretchr/testify/assert"
)

const denom = "uakt"

func TestSettleFullblocks(t *testing.T) {
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
				transferred:  []sdk.Dec{sdk.NewDec(5), sdk.NewDec(10)},
				remaining:    0,
				overdrawn:    false,
			},
		},
		{
			name: "use it all",
			cfg: distTestConfig{
				blocks:       5,
				balanceStart: 100,
				rates:        []int64{10, 10},
				balanceEnd:   0,
				transferred:  []sdk.Dec{sdk.NewDec(50), sdk.NewDec(50)},
				remaining:    0,
				overdrawn:    false,
			},
		},
		{
			name: "ruh roh",
			cfg: distTestConfig{
				blocks:       6,
				balanceStart: 100,
				rates:        []int64{10, 10},
				balanceEnd:   0,
				transferred:  []sdk.Dec{sdk.NewDec(50), sdk.NewDec(50)},
				remaining:    0,
				overdrawn:    true,
			},
		},
		{
			name: "some left",
			cfg: distTestConfig{
				blocks:       6,
				balanceStart: 90,
				rates:        []int64{10, 10},
				balanceEnd:   10,
				transferred:  []sdk.Dec{sdk.NewDec(40), sdk.NewDec(40)},
				remaining:    10,
				overdrawn:    true,
			},
		},
		{
			name: "plenty funds",
			cfg: distTestConfig{
				blocks:       5,
				balanceStart: 0,
				rates:        []int64{1, 2},
				balanceEnd:   0,
				fundsStart:   100,
				fundsEnd:     85,
				transferred:  []sdk.Dec{sdk.NewDec(5), sdk.NewDec(10)},
				remaining:    0,
				overdrawn:    false,
			},
		},
		{
			name: "use all funds",
			cfg: distTestConfig{
				blocks:       5,
				balanceStart: 100,
				rates:        []int64{10, 10},
				balanceEnd:   100,
				fundsStart:   100,
				fundsEnd:     0,
				transferred:  []sdk.Dec{sdk.NewDec(50), sdk.NewDec(50)},
				remaining:    0,
				overdrawn:    false,
			},
		},
		{
			name: "use all funds with some balance",
			cfg: distTestConfig{
				blocks:       6,
				balanceStart: 100,
				rates:        []int64{10, 10},
				balanceEnd:   80,
				fundsStart:   100,
				fundsEnd:     0,
				transferred:  []sdk.Dec{sdk.NewDec(60), sdk.NewDec(60)},
				remaining:    0,
				overdrawn:    false,
			},
		},
		{
			name: "use all funds and balance",
			cfg: distTestConfig{
				blocks:       10,
				balanceStart: 100,
				rates:        []int64{10, 10},
				balanceEnd:   0,
				fundsStart:   100,
				fundsEnd:     0,
				transferred:  []sdk.Dec{sdk.NewDec(100), sdk.NewDec(100)},
				remaining:    0,
				overdrawn:    false,
			},
		},
		{
			name: "overdrawn with all funds and balance used up",
			cfg: distTestConfig{
				blocks:       11,
				balanceStart: 100,
				rates:        []int64{10, 10},
				balanceEnd:   0,
				fundsStart:   100,
				fundsEnd:     0,
				transferred:  []sdk.Dec{sdk.NewDec(100), sdk.NewDec(100)},
				remaining:    0,
				overdrawn:    true,
			},
		},
		{
			name: "overdrawn with all funds used and some balance left",
			cfg: distTestConfig{
				blocks:       10,
				balanceStart: 100,
				rates:        []int64{10, 10},
				balanceEnd:   10,
				fundsStart:   90,
				fundsEnd:     0,
				transferred:  []sdk.Dec{sdk.NewDec(90), sdk.NewDec(90)},
				remaining:    10,
				overdrawn:    true,
			},
		},
		{
			name: "overdrawn when only funds",
			cfg: distTestConfig{
				blocks:       6,
				balanceStart: 0,
				rates:        []int64{10, 10},
				balanceEnd:   10,
				fundsStart:   90,
				fundsEnd:     0,
				transferred:  []sdk.Dec{sdk.NewDec(40), sdk.NewDec(40)},
				remaining:    10,
				overdrawn:    true,
			},
		},
	} {
		account, payments, blocks, blockRate := setupDistTest(tt.cfg)

		account, payments, overdrawn, remaining := accountSettleFullblocks(
			account, payments, blocks, blockRate)

		assertCoinsEqual(t, sdk.NewInt64DecCoin(denom, tt.cfg.balanceEnd), account.Balance, tt.name)
		assertCoinsEqual(t, sdk.NewInt64DecCoin(denom, tt.cfg.fundsEnd), account.Funds, tt.name)

		for idx := range payments {
			assert.Equal(t, sdk.NewDecCoinFromDec(denom, tt.cfg.transferred[idx]), payments[idx].Balance, tt.name)
		}

		assertCoinsEqual(t, sdk.NewInt64DecCoin(denom, tt.cfg.remaining), remaining, tt.name)
		assert.Equal(t, tt.cfg.overdrawn, overdrawn, tt.name)
	}
}

func TestSettleDistributeWeighted(t *testing.T) {
	for _, tt := range []struct {
		name string
		cfg  distTestConfig
	}{
		{
			name: "all goes - unbalanced",
			cfg: distTestConfig{
				balanceStart: 10,
				rates:        []int64{20, 30},
				balanceEnd:   0,
				transferred:  []sdk.Dec{sdk.NewDec(4), sdk.NewDec(6)},
				remaining:    0,
				overdrawn:    false,
			},
		},
		{
			name: "all goes - balanced",
			cfg: distTestConfig{
				balanceStart: 10,
				rates:        []int64{30, 30},
				balanceEnd:   0,
				transferred:  []sdk.Dec{sdk.NewDec(5), sdk.NewDec(5)},
				remaining:    0,
				overdrawn:    false,
			},
		},
		{
			name: "all goes - unbalanced",
			cfg: distTestConfig{
				balanceStart: 10,
				rates:        []int64{45, 55},
				balanceEnd:   0,
				transferred:  []sdk.Dec{sdk.MustNewDecFromStr("4.5"), sdk.MustNewDecFromStr("5.5")},
				remaining:    0,
				overdrawn:    false,
			},
		},
	} {
		account, payments, _, blockRate := setupDistTest(tt.cfg)

		account, payments, remaining := accountSettleDistributeWeighted(
			account, payments, blockRate, account.Balance)

		assertCoinsEqual(t, sdk.NewInt64DecCoin(denom, tt.cfg.balanceEnd), account.Balance, tt.name)

		for idx := range payments {
			assert.Equal(t, sdk.NewDecCoinFromDec(denom, tt.cfg.transferred[idx]), payments[idx].Balance, tt.name)
		}

		assertCoinsEqual(t, sdk.NewInt64DecCoin(denom, tt.cfg.remaining), remaining, tt.name)
	}
}

type distTestConfig struct {
	blocks       int64
	balanceStart int64
	rates        []int64
	balanceEnd   int64
	fundsStart   int64
	fundsEnd     int64
	transferred  []sdk.Dec
	remaining    int64
	overdrawn    bool
}

func setupDistTest(cfg distTestConfig) (types.Account, []types.FractionalPayment, sdk.Int, sdk.DecCoin) {
	account := types.Account{
		Balance:     sdk.NewInt64DecCoin(denom, cfg.balanceStart),
		Transferred: sdk.NewInt64DecCoin(denom, 0),
		Funds:       sdk.NewInt64DecCoin(denom, cfg.fundsStart),
	}

	payments := make([]types.FractionalPayment, 0, len(cfg.rates))

	blockRate := int64(0)

	for _, rate := range cfg.rates {
		blockRate += rate
		payments = append(payments, types.FractionalPayment{
			Rate:    sdk.NewInt64DecCoin(denom, rate),
			Balance: sdk.NewInt64DecCoin(denom, 0),
		})
	}

	return account, payments, sdk.NewInt(cfg.blocks), sdk.NewInt64DecCoin(denom, blockRate)
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
