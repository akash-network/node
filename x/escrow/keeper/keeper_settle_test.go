package keeper

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	etypes "pkg.akt.dev/go/node/escrow/types/v1"
	"pkg.akt.dev/go/testutil"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/assert"
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
			},
		},
		{
			name: "ruh roh",
			cfg: distTestConfig{
				blocks:       6,
				balanceStart: 100,
				rates:        []int64{10, 10},
				balanceEnd:   -20,
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(60), sdkmath.LegacyNewDec(40)},
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
			},
		},
		{
			name: "overdrawn with all funds and balance used up",
			cfg: distTestConfig{
				blocks:       11,
				balanceStart: 200,
				rates:        []int64{10, 10},
				balanceEnd:   -20,
				transferred:  []sdkmath.LegacyDec{sdkmath.LegacyNewDec(110), sdkmath.LegacyNewDec(90)},
			},
		},
	} {
		account, payments, blocks := setupDistTest(t, tt.cfg)

		overdrawn := accountSettleFullBlocks(&account, payments, blocks)
		assert.Equal(t, tt.cfg.balanceEnd < 0, overdrawn, tt.name)

		assertAmountEqual(t, sdkmath.LegacyNewDec(tt.cfg.balanceEnd), account.State.Funds[0].Amount, tt.name)

		for idx := range payments {
			assert.Equal(t, sdk.NewDecCoinFromDec(denom, tt.cfg.transferred[idx]), payments[idx].State.Balance, tt.name)
		}
	}
}

type distTestConfig struct {
	blocks       int64
	balanceStart int64
	rates        []int64
	balanceEnd   int64
	transferred  []sdkmath.LegacyDec
}

func setupDistTest(t *testing.T, cfg distTestConfig) (account, []payment, sdkmath.Int) {
	account := account{
		Account: etypes.Account{
			State: etypes.AccountState{
				Funds: []etypes.Balance{
					{
						Denom:  denom,
						Amount: sdkmath.LegacyNewDec(cfg.balanceStart),
					},
				},
				Transferred: sdk.DecCoins{
					sdk.NewInt64DecCoin(denom, 0),
				},
				Deposits: []etypes.Depositor{
					{
						Owner:   testutil.AccAddress(t).String(),
						Height:  0,
						Balance: sdk.NewDecCoinFromCoin(sdk.NewInt64Coin(denom, cfg.balanceStart)),
					},
				},
			},
		},
	}

	payments := make([]payment, 0, len(cfg.rates))

	blockRate := int64(0)

	for _, rate := range cfg.rates {
		blockRate += rate
		payments = append(payments, payment{
			Payment: etypes.Payment{
				State: etypes.PaymentState{
					Rate:      sdk.NewInt64DecCoin(denom, rate),
					Balance:   sdk.NewInt64DecCoin(denom, 0),
					Unsettled: sdk.NewInt64DecCoin(denom, 0),
				},
			},
		})
	}

	return account, payments, sdkmath.NewInt(cfg.blocks)
}

func assertAmountEqual(t testing.TB, c1 sdkmath.LegacyDec, c2 sdkmath.LegacyDec, msg string) {
	t.Helper()
	if c1.IsZero() {
		if !c2.IsZero() {
			assert.Failf(t, msg, "%v is not zero", c2)
		}
		return
	}
	assert.Equal(t, c1, c2, msg)
}
