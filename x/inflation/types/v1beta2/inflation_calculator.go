package v1beta2

import (
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	"github.com/ericlagergren/decimal"
)

func GetInflationCalculator(
	genesisTime time.Time,
	inflationParamSubspace paramstypes.Subspace,
) minttypes.InflationCalculationFn {
	return func(ctx sdk.Context, minter minttypes.Minter, params minttypes.Params, bondedRatio sdk.Dec) sdk.Dec {
		var inflationParams Params
		inflationParamSubspace.GetParamSet(ctx, &inflationParams)

		// years passed since genesis = seconds passed since genesis / number of seconds per year
		// can be a fraction, eg: 0.5
		yearsPassed := decimal.WithPrecision(sdk.Precision).
			Quo(
				// seconds since genesis
				decimal.WithPrecision(sdk.Precision).
					Sub(
						decimal.New(ctx.BlockTime().Unix(), 0),
						decimal.New(genesisTime.Unix(), 0),
					),
				// Number of hours in an year = 8766 (averaging the leap year hours)
				// Number of minutes in an hour = 60
				// Number of seconds in a minute = 60
				// => Number of seconds per year = 60 * 60 * 8766 = 31557600
				decimal.New(31557600, 0),
			)
		// 2^(-t/tHalf)
		pow := decimal.WithPrecision(sdk.Precision)
		pow = pow.Context.
			Pow(
				pow,
				decimal.New(2, 0),
				decimal.WithPrecision(sdk.Precision).
					Mul(
						decimal.New(-1, 0),
						decimal.WithPrecision(sdk.Precision).
							Quo(
								yearsPassed,
								decimal.New(int64(inflationParams.InflationDecayFactor), 0),
							),
					),
			)
		idealInflation := inflationParams.InitialInflation.Mul(sdk.MustNewDecFromStr(pow.String()))

		// (1 - bondedRatio/GoalBonded) * InflationRateChange
		inflationRateChangePerYear := sdk.OneDec().
			Sub(bondedRatio.Quo(params.GoalBonded)).
			Mul(params.InflationRateChange)
		inflationRateChange := inflationRateChangePerYear.Quo(sdk.NewDec(int64(params.BlocksPerYear)))

		// note inflationRateChange may be negative
		currentInflation := minter.Inflation.Add(inflationRateChange)

		// min, max currentInflation based on a defined range parameter 'r'
		// currentInflation range = [I(t) - I(t) * R, I(t) + I(t) * R]
		r := inflationParams.Variance
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
}
