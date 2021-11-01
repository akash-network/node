package v1beta2

import (
	"math"
	"strconv"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
)

var (
	// these are set during NewApp and used in InflationCalculator

	GenesisTime            time.Time
	InflationParamSubspace paramstypes.Subspace
)

func InflationCalculator(ctx sdk.Context, minter minttypes.Minter, params minttypes.Params, bondedRatio sdk.Dec) sdk.Dec {
	var inflationParams Params
	InflationParamSubspace.GetParamSet(ctx, &inflationParams)
	tHalf := float64(inflationParams.InflationDecayFactor)
	initialInflation, err := strconv.ParseFloat(inflationParams.InitialInflation, 32)
	if err != nil {
		panic(err)
	}

	// Number of hours in an year = 8766 (averaging the leap year hours)
	// Number of minutes in an hour = 60
	// Number of seconds in a minute = 60
	// => 60 * 60 * 8766 = Number of seconds per year
	t := ctx.BlockTime().Sub(GenesisTime).Seconds() / (60 * 60 * 8766) // years passed
	i := initialInflation * math.Pow(2, -t/tHalf)
	idealInflation := sdk.NewDec(int64(i))

	// (1 - bondedRatio/GoalBonded) * InflationRateChange
	inflationRateChangePerYear := sdk.OneDec().
		Sub(bondedRatio.Quo(params.GoalBonded)).
		Mul(params.InflationRateChange)
	inflationRateChange := inflationRateChangePerYear.Quo(sdk.NewDec(int64(params.BlocksPerYear)))

	// note inflationRateChange may be negative
	currentInflation := minter.Inflation.Add(inflationRateChange)

	// min, max currentInflation based on a defined range parameter 'r'
	// currentInflation range = [I(t) - I(t) * R, I(t) + I(t) * R]
	r, err := sdk.NewDecFromStr(inflationParams.Variance)
	if err != nil {
		panic(err)
	}
	minInflation := idealInflation.Sub(idealInflation.Mul(r))
	maxInflation := idealInflation.Add(idealInflation.Mul(r))

	// minInflation >= minimumMinInflation
	minimumMinInflation := sdk.ZeroDec() // 0 for now
	if minInflation.LT(minimumMinInflation) {
		minInflation = minimumMinInflation
	}

	if currentInflation.LT(minInflation) {
		currentInflation = minInflation
	} else if currentInflation.GT(maxInflation) {
		currentInflation = maxInflation
	}

	return currentInflation
}
