package keeper

import (
	"testing"

	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	"cosmossdk.io/store"
	"cosmossdk.io/store/metrics"
	storetypes "cosmossdk.io/store/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/take/v1"
)

func setupKeeper(t *testing.T) (Keeper, sdk.Context) {
	t.Helper()

	storeKey := storetypes.NewKVStoreKey("take")

	db := dbm.NewMemDB()
	stateStore := store.NewCommitMultiStore(db, log.NewNopLogger(), metrics.NewNoOpMetrics())
	stateStore.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	if err := stateStore.LoadLatestVersion(); err != nil {
		t.Fatal(err)
	}

	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)

	authority := "akash10d07y265gmmuvt4z0w9aw880jnsr700j35yzgl"

	keeper := NewKeeper(cdc, storeKey, authority).(Keeper)

	ctx := sdk.NewContext(stateStore, cmtproto.Header{}, false, log.NewNopLogger())

	return keeper, ctx
}

func TestCodec(t *testing.T) {
	keeper, _ := setupKeeper(t)

	cdc := keeper.Codec()
	if cdc == nil {
		t.Fatal("Codec() returned nil")
	}
}

func TestStoreKey(t *testing.T) {
	keeper, _ := setupKeeper(t)

	storeKey := keeper.StoreKey()
	if storeKey == nil {
		t.Fatal("StoreKey() returned nil")
	}

	if storeKey.Name() != "take" {
		t.Errorf("StoreKey().Name() = %v, want %v", storeKey.Name(), "take")
	}
}

func TestNewQuerier(t *testing.T) {
	keeper, _ := setupKeeper(t)

	querier := keeper.NewQuerier()
	if querier.Keeper.authority == "" {
		t.Fatal("NewQuerier() returned invalid Querier")
	}
}

func TestGetAuthority(t *testing.T) {
	keeper, _ := setupKeeper(t)

	authority := keeper.GetAuthority()
	if authority == "" {
		t.Fatal("GetAuthority() returned empty string")
	}

	if authority != "akash10d07y265gmmuvt4z0w9aw880jnsr700j35yzgl" {
		t.Errorf("GetAuthority() = %v, want akash10d07y265gmmuvt4z0w9aw880jnsr700j35yzgl", authority)
	}
}

func TestSetAndGetParams(t *testing.T) {
	keeper, ctx := setupKeeper(t)

	tests := []struct {
		name    string
		params  types.Params
		wantErr bool
	}{
		{
			name: "default params",
			params: types.Params{
				DefaultTakeRate: 20,
				DenomTakeRates: types.DenomTakeRates{
					{Denom: "uakt", Rate: 2},
				},
			},
			wantErr: false,
		},
		{
			name: "zero default rate",
			params: types.Params{
				DefaultTakeRate: 0,
				DenomTakeRates: types.DenomTakeRates{
					{Denom: "uakt", Rate: 5},
				},
			},
			wantErr: false,
		},
		{
			name: "max rate",
			params: types.Params{
				DefaultTakeRate: 100,
				DenomTakeRates: types.DenomTakeRates{
					{Denom: "uakt", Rate: 100},
				},
			},
			wantErr: false,
		},
		{
			name: "multiple denoms",
			params: types.Params{
				DefaultTakeRate: 10,
				DenomTakeRates: types.DenomTakeRates{
					{Denom: "uakt", Rate: 2},
					{Denom: "usdc", Rate: 5},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid rate over 100",
			params: types.Params{
				DefaultTakeRate: 101,
				DenomTakeRates: types.DenomTakeRates{
					{Denom: "uakt", Rate: 2},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid denom rate over 100",
			params: types.Params{
				DefaultTakeRate: 50,
				DenomTakeRates: types.DenomTakeRates{
					{Denom: "uakt", Rate: 101},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid missing uakt",
			params: types.Params{
				DefaultTakeRate: 20,
				DenomTakeRates: types.DenomTakeRates{
					{Denom: "usdc", Rate: 5},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid duplicate denom",
			params: types.Params{
				DefaultTakeRate: 20,
				DenomTakeRates: types.DenomTakeRates{
					{Denom: "uakt", Rate: 2},
					{Denom: "uakt", Rate: 5},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := keeper.SetParams(ctx, tt.params)

			if tt.wantErr {
				if err == nil {
					t.Error("SetParams() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("SetParams() unexpected error: %v", err)
				return
			}

			got := keeper.GetParams(ctx)

			if got.DefaultTakeRate != tt.params.DefaultTakeRate {
				t.Errorf("GetParams().DefaultTakeRate = %v, want %v", got.DefaultTakeRate, tt.params.DefaultTakeRate)
			}

			if len(got.DenomTakeRates) != len(tt.params.DenomTakeRates) {
				t.Errorf("GetParams().DenomTakeRates length = %v, want %v", len(got.DenomTakeRates), len(tt.params.DenomTakeRates))
				return
			}

			for i := range got.DenomTakeRates {
				if got.DenomTakeRates[i].Denom != tt.params.DenomTakeRates[i].Denom {
					t.Errorf("GetParams().DenomTakeRates[%d].Denom = %v, want %v", i, got.DenomTakeRates[i].Denom, tt.params.DenomTakeRates[i].Denom)
				}
				if got.DenomTakeRates[i].Rate != tt.params.DenomTakeRates[i].Rate {
					t.Errorf("GetParams().DenomTakeRates[%d].Rate = %v, want %v", i, got.DenomTakeRates[i].Rate, tt.params.DenomTakeRates[i].Rate)
				}
			}
		})
	}
}

func TestGetParamsEmpty(t *testing.T) {
	keeper, ctx := setupKeeper(t)

	params := keeper.GetParams(ctx)

	if params.DefaultTakeRate != 0 {
		t.Errorf("GetParams() on empty store should return zero values, got DefaultTakeRate = %v", params.DefaultTakeRate)
	}

	if len(params.DenomTakeRates) != 0 {
		t.Errorf("GetParams() on empty store should return empty DenomTakeRates, got length = %v", len(params.DenomTakeRates))
	}
}

func TestSubtractFees(t *testing.T) {
	keeper, ctx := setupKeeper(t)

	err := keeper.SetParams(ctx, types.Params{
		DefaultTakeRate: 20,
		DenomTakeRates: types.DenomTakeRates{
			{Denom: "uakt", Rate: 2},
			{Denom: "usdc", Rate: 5},
		},
	})
	if err != nil {
		t.Fatalf("SetParams() failed: %v", err)
	}

	tests := []struct {
		name            string
		amount          sdk.Coin
		expectedEarning sdk.Coin
		expectedFee     sdk.Coin
	}{
		{
			name:            "uakt with 2% rate",
			amount:          sdk.NewInt64Coin("uakt", 1000),
			expectedEarning: sdk.NewInt64Coin("uakt", 980),
			expectedFee:     sdk.NewInt64Coin("uakt", 20),
		},
		{
			name:            "uakt with small amount",
			amount:          sdk.NewInt64Coin("uakt", 10),
			expectedEarning: sdk.NewInt64Coin("uakt", 10),
			expectedFee:     sdk.NewInt64Coin("uakt", 0),
		},
		{
			name:            "uakt with zero amount",
			amount:          sdk.NewInt64Coin("uakt", 0),
			expectedEarning: sdk.NewInt64Coin("uakt", 0),
			expectedFee:     sdk.NewInt64Coin("uakt", 0),
		},
		{
			name:            "usdc with 5% rate",
			amount:          sdk.NewInt64Coin("usdc", 1000),
			expectedEarning: sdk.NewInt64Coin("usdc", 950),
			expectedFee:     sdk.NewInt64Coin("usdc", 50),
		},
		{
			name:            "usdc with fractional fee",
			amount:          sdk.NewInt64Coin("usdc", 999),
			expectedEarning: sdk.NewInt64Coin("usdc", 950),
			expectedFee:     sdk.NewInt64Coin("usdc", 49),
		},
		{
			name:            "unknown denom uses default 20% rate",
			amount:          sdk.NewInt64Coin("atom", 1000),
			expectedEarning: sdk.NewInt64Coin("atom", 800),
			expectedFee:     sdk.NewInt64Coin("atom", 200),
		},
		{
			name:            "large amount",
			amount:          sdk.NewInt64Coin("uakt", 1000000),
			expectedEarning: sdk.NewInt64Coin("uakt", 980000),
			expectedFee:     sdk.NewInt64Coin("uakt", 20000),
		},
		{
			name:            "amount of 1",
			amount:          sdk.NewInt64Coin("uakt", 1),
			expectedEarning: sdk.NewInt64Coin("uakt", 1),
			expectedFee:     sdk.NewInt64Coin("uakt", 0),
		},
		{
			name:            "amount of 50 with 2% rate",
			amount:          sdk.NewInt64Coin("uakt", 50),
			expectedEarning: sdk.NewInt64Coin("uakt", 49),
			expectedFee:     sdk.NewInt64Coin("uakt", 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			earning, fee, err := keeper.SubtractFees(ctx, tt.amount)

			if err != nil {
				t.Errorf("SubtractFees() unexpected error: %v", err)
				return
			}

			if !earning.Equal(tt.expectedEarning) {
				t.Errorf("SubtractFees() earning = %v, want %v", earning, tt.expectedEarning)
			}

			if !fee.Equal(tt.expectedFee) {
				t.Errorf("SubtractFees() fee = %v, want %v", fee, tt.expectedFee)
			}

			total := earning.Add(fee)
			if !total.Equal(tt.amount) {
				t.Errorf("SubtractFees() earning + fee = %v, want %v", total, tt.amount)
			}
		})
	}
}

func TestFindRate(t *testing.T) {
	keeper, ctx := setupKeeper(t)

	err := keeper.SetParams(ctx, types.Params{
		DefaultTakeRate: 15,
		DenomTakeRates: types.DenomTakeRates{
			{Denom: "uakt", Rate: 2},
			{Denom: "usdc", Rate: 10},
		},
	})
	if err != nil {
		t.Fatalf("SetParams() failed: %v", err)
	}

	tests := []struct {
		name         string
		denom        string
		expectedRate sdkmath.LegacyDec
	}{
		{
			name:         "uakt specific rate",
			denom:        "uakt",
			expectedRate: sdkmath.LegacyMustNewDecFromStr("0.02"),
		},
		{
			name:         "usdc specific rate",
			denom:        "usdc",
			expectedRate: sdkmath.LegacyMustNewDecFromStr("0.10"),
		},
		{
			name:         "empty denom uses default",
			denom:        "",
			expectedRate: sdkmath.LegacyMustNewDecFromStr("0.15"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rate := keeper.findRate(ctx, tt.denom)

			if !rate.Equal(tt.expectedRate) {
				t.Errorf("findRate(%q) = %v, want %v", tt.denom, rate, tt.expectedRate)
			}
		})
	}
}

func TestFindRateWithNoParams(t *testing.T) {
	keeper, ctx := setupKeeper(t)

	rate := keeper.findRate(ctx, "uakt")

	expectedRate := sdkmath.LegacyMustNewDecFromStr("0.00")
	if !rate.Equal(expectedRate) {
		t.Errorf("findRate() with no params = %v, want %v", rate, expectedRate)
	}
}

func TestSubtractFeesWithZeroRate(t *testing.T) {
	keeper, ctx := setupKeeper(t)

	err := keeper.SetParams(ctx, types.Params{
		DefaultTakeRate: 0,
		DenomTakeRates: types.DenomTakeRates{
			{Denom: "uakt", Rate: 0},
		},
	})
	if err != nil {
		t.Fatalf("SetParams() failed: %v", err)
	}

	amount := sdk.NewInt64Coin("uakt", 1000)
	earning, fee, err := keeper.SubtractFees(ctx, amount)

	if err != nil {
		t.Errorf("SubtractFees() unexpected error: %v", err)
	}

	if !earning.Equal(amount) {
		t.Errorf("SubtractFees() with zero rate earning = %v, want %v", earning, amount)
	}

	expectedFee := sdk.NewInt64Coin("uakt", 0)
	if !fee.Equal(expectedFee) {
		t.Errorf("SubtractFees() with zero rate fee = %v, want %v", fee, expectedFee)
	}
}

func TestSubtractFeesWithMaxRate(t *testing.T) {
	keeper, ctx := setupKeeper(t)

	err := keeper.SetParams(ctx, types.Params{
		DefaultTakeRate: 100,
		DenomTakeRates: types.DenomTakeRates{
			{Denom: "uakt", Rate: 100},
		},
	})
	if err != nil {
		t.Fatalf("SetParams() failed: %v", err)
	}

	amount := sdk.NewInt64Coin("uakt", 1000)
	earning, fee, err := keeper.SubtractFees(ctx, amount)

	if err != nil {
		t.Errorf("SubtractFees() unexpected error: %v", err)
	}

	expectedEarning := sdk.NewInt64Coin("uakt", 0)
	if !earning.Equal(expectedEarning) {
		t.Errorf("SubtractFees() with 100%% rate earning = %v, want %v", earning, expectedEarning)
	}

	if !fee.Equal(amount) {
		t.Errorf("SubtractFees() with 100%% rate fee = %v, want %v", fee, amount)
	}
}
