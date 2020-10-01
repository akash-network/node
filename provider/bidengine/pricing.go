package bidengine

import (
	"crypto/rand"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/types/unit"
	"github.com/ovrclk/akash/validation"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

func calculatePrice(gspec *dtypes.GroupSpec) (sdk.Coin, error) {

	min, max := calculatePriceRange(gspec)

	if min.IsEqual(max) {
		return max, nil
	}

	delta := max.Amount.Sub(min.Amount)

	val, err := rand.Int(rand.Reader, delta.BigInt())
	if err != nil {
		return sdk.Coin{}, err
	}

	return sdk.NewCoin(min.Denom, min.Amount.Add(sdk.NewIntFromBigInt(val))), nil
}

func calculatePriceRange(gspec *dtypes.GroupSpec) (sdk.Coin, sdk.Coin) {
	// memory-based pricing:
	//   min: requested memory * configured min price per Gi
	//   max: requested memory * configured max price per Gi

	// assumption: group.Count > 0
	// assumption: all same denom (returned by gspec.Price())
	// assumption: gspec.Price() > 0

	mem := sdk.NewInt(0)

	cfg := validation.Config()

	for _, group := range gspec.Resources {
		mem = mem.Add(
			sdk.NewIntFromUint64(group.Resources.Memory.Quantity.Value()).
				MulRaw(int64(group.Count)))
	}

	rmax := gspec.Price()

	cmin := mem.MulRaw(
		cfg.MinGroupMemPrice).
		Quo(sdk.NewInt(unit.Gi))

	cmax := mem.MulRaw(
		cfg.MaxGroupMemPrice).
		Quo(sdk.NewInt(unit.Gi))

	if cmax.GT(rmax.Amount) {
		cmax = rmax.Amount
	}

	if cmax.IsZero() {
		cmax = sdk.NewInt(1)
	}

	return sdk.NewCoin(rmax.Denom, cmin), sdk.NewCoin(rmax.Denom, cmax)
}
