package v1beta2

// todo akash-network/support#4
// import (
// 	"fmt"
// 	"time"
//
// 	sdk "github.com/cosmos/cosmos-sdk/types"
// 	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
// 	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
// 	"github.com/ericlagergren/decimal"
// )
//
// func GetInflationCalculator(
// 	genesisTime time.Time,
// 	inflationParamSubspace paramstypes.Subspace,
// ) minttypes.InflationCalculationFn {
// 	return func(ctx sdk.Context, minter minttypes.Minter, params minttypes.Params, bondedRatio sdk.Dec) sdk.Dec {
// 		var inflationParams Params
// 		inflationParamSubspace.GetParamSet(ctx, &inflationParams)
//
// 		return inflationCalculator(ctx.BlockTime(), genesisTime, minter, params, inflationParams, bondedRatio)
// 	}
// }
//
// // inflationCalculator calculate current inflation value
// // - btime - block time from sdk.Context
// // - gtime - genesis time
// func inflationCalculator(btime, gtime time.Time, minter minttypes.Minter, mparams minttypes.Params, iparams Params, bondedRatio sdk.Dec) sdk.Dec {
// 	inflationDecayFactor := new(decimal.Big)
// 	if _, valid := inflationDecayFactor.SetString(iparams.InflationDecayFactor.String()); !valid {
// 		panic(fmt.Sprintf("InflationDecayFactor contains invalid value [%s]. expected integer/float", iparams.InflationDecayFactor.String()))
// 	}
//
// 	// years passed since genesis = seconds passed since genesis / number of seconds per year
// 	// can be a fraction, eg: 0.5
// 	yearsPassed := decimal.WithPrecision(sdk.Precision).
// 		Quo(
// 			// seconds since genesis
// 			decimal.WithPrecision(sdk.Precision).
// 				Sub(
// 					decimal.New(btime.Unix(), 0),
// 					decimal.New(gtime.Unix(), 0),
// 				),
// 			// Number of hours in an year = 8766 (averaging the leap year hours)
// 			// Number of minutes in an hour = 60
// 			// Number of seconds in a minute = 60
// 			// => Number of seconds per year = 60 * 60 * 8766 = 31557600
// 			decimal.New(31557600, 0),
// 		)
// 	// 2^(-t/tHalf)
// 	inflationCoefDec := decimal.WithPrecision(sdk.Precision)
// 	inflationCoefDec = inflationCoefDec.Context.
// 		Pow(
// 			inflationCoefDec,
// 			decimal.New(2, 0),
// 			decimal.WithPrecision(sdk.Precision).
// 				Mul(
// 					decimal.New(-1, 0),
// 					decimal.WithPrecision(sdk.Precision).
// 						Quo(yearsPassed, inflationDecayFactor),
// 				),
// 		)
// 	// convert inflationCoefDec to sdk.Dec with a 6 unit precision: sdk.Decimal(big.Int(pow * 10^6)) / 10^6
// 	inflationCoef := sdk.NewDecFromBigInt(
// 		decimal.WithPrecision(sdk.Precision).
// 			Mul(inflationCoefDec, decimal.New(1000000, 0)).
// 			Int(nil),
// 	).QuoInt64(1000000)
//
// 	idealInflation := iparams.InitialInflation.Mul(inflationCoef)
//
// 	// (1 - bondedRatio/GoalBonded) * InflationRateChange
// 	inflationRateChangePerYear := sdk.OneDec().
// 		Sub(bondedRatio.Quo(mparams.GoalBonded)).
// 		Mul(mparams.InflationRateChange)
//
// 	inflationRateChange := inflationRateChangePerYear.Quo(sdk.NewDecFromInt(sdk.NewIntFromUint64(mparams.BlocksPerYear)))
//
// 	sdk.NewDecFromInt(sdk.NewIntFromUint64(mparams.BlocksPerYear))
//
// 	// note inflationRateChange may be negative
// 	currentInflation := minter.Inflation.Add(inflationRateChange)
//
// 	// min, max currentInflation based on a defined range parameter 'r'
// 	// currentInflation range = [I(t) - I(t) * R, I(t) + I(t) * R]
// 	// R is from iparams.Variance
// 	minInflation := idealInflation.Sub(idealInflation.Mul(iparams.Variance))
// 	maxInflation := idealInflation.Add(idealInflation.Mul(iparams.Variance))
//
// 	// the lowest possible value of minInflation is set for 0
// 	// tho it can be set to higher value in the future
// 	minInflation = sdk.MaxDec(sdk.ZeroDec(), minInflation)
//
// 	if currentInflation.LT(minInflation) {
// 		currentInflation = minInflation
// 	} else if currentInflation.GT(maxInflation) {
// 		currentInflation = maxInflation
// 	}
//
// 	return currentInflation
// }
