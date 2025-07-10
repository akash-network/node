package keeper

import (
	"testing"

	sdkmath "cosmossdk.io/math"

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
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(50), sdkmath.LegacyNewDec(50)},
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
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(50), sdkmath.LegacyNewDec(50)},
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
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(40), sdkmath.LegacyNewDec(40)},
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
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(5), sdkmath.LegacyNewDec(10)},
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
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(50), sdkmath.LegacyNewDec(50)},
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
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(60), sdkmath.LegacyNewDec(60)},
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
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(100), sdkmath.LegacyNewDec(100)},
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
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(100), sdkmath.LegacyNewDec(100)},
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
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(90), sdkmath.LegacyNewDec(90)},
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
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(40), sdkmath.LegacyNewDec(40)},
				remaining:    10,
				overdrawn:    true,
			},
		},
	} {
		account, payments, blocks, blockRate := setupDistTest(tt.cfg)

		account, payments, overdrawn, remaining := accountSettleFullBlocks(
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
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(4), sdkmath.LegacyNewDec(6)},
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
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(5), sdkmath.LegacyNewDec(5)},
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
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyMustNewDecFromStr("4.5"), sdkmath.LegacyMustNewDecFromStr("5.5")},
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
	transferred  []sdkmath.LegacyDec
	remaining    int64
	overdrawn    bool
}

func setupDistTest(cfg distTestConfig) (v1.Account, []v1.FractionalPayment, sdkmath.Int, sdk.DecCoin) {
	account := v1.Account{
		Balance:     sdk.NewInt64DecCoin(denom, cfg.balanceStart),
		Transferred: sdk.NewInt64DecCoin(denom, 0),
		Funds:       sdk.NewInt64DecCoin(denom, cfg.fundsStart),
	}

	payments := make([]v1.FractionalPayment, 0, len(cfg.rates))

	blockRate := int64(0)

	for _, rate := range cfg.rates {
		blockRate += rate
		payments = append(payments, v1.FractionalPayment{
			Rate:    sdk.NewInt64DecCoin(denom, rate),
			Balance: sdk.NewInt64DecCoin(denom, 0),
		})
	}

	return account, payments, sdkmath.NewInt(cfg.blocks), sdk.NewInt64DecCoin(denom, blockRate)
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
