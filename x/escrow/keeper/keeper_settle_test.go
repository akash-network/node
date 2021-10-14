package keeper

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	types "github.com/ovrclk/akash/x/escrow/types/v1beta2"
	"github.com/stretchr/testify/assert"
)

const denom = "xxx"

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
				transferred:  []int64{5, 10},
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
				transferred:  []int64{50, 50},
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
				transferred:  []int64{50, 50},
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
				transferred:  []int64{40, 40},
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
				transferred:  []int64{5, 10},
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
				transferred:  []int64{50, 50},
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
				transferred:  []int64{60, 60},
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
				transferred:  []int64{100, 100},
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
				transferred:  []int64{100, 100},
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
				transferred:  []int64{90, 90},
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
				transferred:  []int64{40, 40},
				remaining:    10,
				overdrawn:    true,
			},
		},
	} {
		account, payments, blocks, blockRate := setupDistTest(tt.cfg)

		account, payments, overdrawn, remaining := accountSettleFullblocks(
			account, payments, blocks, blockRate)

		assertCoinsEqual(t, sdk.NewInt64Coin(denom, tt.cfg.balanceEnd), account.Balance, tt.name)
		assertCoinsEqual(t, sdk.NewInt64Coin(denom, tt.cfg.fundsEnd), account.Funds, tt.name)

		for idx := range payments {
			assert.Equal(t, sdk.NewInt64Coin(denom, tt.cfg.transferred[idx]), payments[idx].Balance, tt.name)
		}

		assertCoinsEqual(t, sdk.NewInt64Coin(denom, tt.cfg.remaining), remaining, tt.name)
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
				transferred:  []int64{4, 6},
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
				transferred:  []int64{5, 5},
				remaining:    0,
				overdrawn:    false,
			},
		},
		{
			name: "some left - unbalanced",
			cfg: distTestConfig{
				balanceStart: 10,
				rates:        []int64{45, 55},
				balanceEnd:   1,
				transferred:  []int64{4, 5},
				remaining:    1,
				overdrawn:    false,
			},
		},
	} {
		account, payments, _, blockRate := setupDistTest(tt.cfg)

		account, payments, remaining := accountSettleDistributeWeighted(
			account, payments, blockRate, account.Balance)

		assertCoinsEqual(t, sdk.NewInt64Coin(denom, tt.cfg.balanceEnd), account.Balance, tt.name)

		for idx := range payments {
			assert.Equal(t, sdk.NewInt64Coin(denom, tt.cfg.transferred[idx]), payments[idx].Balance, tt.name)
		}

		assertCoinsEqual(t, sdk.NewInt64Coin(denom, tt.cfg.remaining), remaining, tt.name)
	}
}

func TestSettleDistributeEvenly(t *testing.T) {
	for _, tt := range []struct {
		name string
		cfg  distTestConfig
	}{
		{
			name: "even",
			cfg: distTestConfig{
				balanceStart: 2,
				rates:        []int64{20, 30},
				balanceEnd:   0,
				transferred:  []int64{1, 1},
			},
		},
		{
			name: "not even",
			cfg: distTestConfig{
				balanceStart: 3,
				rates:        []int64{20, 30},
				balanceEnd:   0,
				transferred:  []int64{2, 1},
			},
		},
	} {
		account, payments, _, _ := setupDistTest(tt.cfg)

		account, payments, remaining := accountSettleDistributeEvenly(
			account, payments, account.Balance)

		assertCoinsEqual(t, sdk.NewInt64Coin(denom, tt.cfg.balanceEnd), account.Balance, tt.name)

		for idx := range payments {
			assert.Equal(t, sdk.NewInt64Coin(denom, tt.cfg.transferred[idx]), payments[idx].Balance, tt.name)
		}

		assertCoinsEqual(t, sdk.NewInt64Coin(denom, tt.cfg.remaining), remaining, tt.name)
	}
}

type distTestConfig struct {
	blocks       int64
	balanceStart int64
	rates        []int64
	balanceEnd   int64
	fundsStart   int64
	fundsEnd     int64
	transferred  []int64
	remaining    int64
	overdrawn    bool
}

func setupDistTest(cfg distTestConfig) (types.Account, []types.Payment, sdk.Int, sdk.Coin) {
	account := types.Account{
		Balance:     sdk.NewInt64Coin(denom, cfg.balanceStart),
		Transferred: sdk.NewInt64Coin(denom, 0),
		Funds:       sdk.NewInt64Coin(denom, cfg.fundsStart),
	}

	payments := make([]types.Payment, 0, len(cfg.rates))

	blockRate := int64(0)

	for _, rate := range cfg.rates {
		blockRate += rate
		payments = append(payments, types.Payment{
			Rate:    sdk.NewInt64Coin(denom, rate),
			Balance: sdk.NewInt64Coin(denom, 0),
		})
	}

	return account, payments, sdk.NewInt(cfg.blocks), sdk.NewInt64Coin(denom, blockRate)
}

func assertCoinsEqual(t testing.TB, c1 sdk.Coin, c2 sdk.Coin, msg string) {
	t.Helper()
	if c1.IsZero() {
		if !c2.IsZero() {
			assert.Failf(t, msg, "%v is not zero", c2)
		}
		return
	}
	assert.Equal(t, c1, c2, msg)
}
