package testutil

import (
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/tendermint/tendermint/libs/rand"

	"github.com/ovrclk/akash/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

func init() {
	config := sdk.GetConfig()
	config.SetBech32PrefixForAccount(BechPrefix, BechPrefix)
	config.Seal()
}

// CoinDenom provides ability to create coins in test functions and
// pass them into testutil functionality.
const (
	CoinDenom  = "uakt"
	BechPrefix = "akash"
)

// Name generates a random name with the given prefix
func Name(_ testing.TB, prefix string) string {
	return fmt.Sprintf("%s-%v", prefix, rand.Uint64())
}

// Hostname generates a random hostname with a "test.com" domain
func Hostname(t testing.TB) string {
	return Name(t, "hostname") + ".test.com"
}

func ProviderHostname(t testing.TB) string {
	return "https://" + Hostname(t)
}

// Attribute generates a random sdk.Attribute
func Attribute(t testing.TB) types.Attribute {
	t.Helper()
	return types.NewStringAttribute(Name(t, "attr-key"), Name(t, "attr-value"))
}

// Attributes generates a set of sdk.Attribute
func Attributes(t testing.TB) []types.Attribute {
	t.Helper()
	count := rand.Intn(10) + 1

	vals := make([]types.Attribute, 0, count)
	for i := 0; i < count; i++ {
		vals = append(vals, Attribute(t))
	}
	return vals
}

// PlacementRequirements generates placement requirements
func PlacementRequirements(t testing.TB) types.PlacementRequirements {
	return types.PlacementRequirements{
		Attributes: Attributes(t),
	}
}

func RandCPUUnits() uint {
	return RandRangeUint(
		dtypes.GetValidationConfig().MinUnitCPU,
		dtypes.GetValidationConfig().MaxUnitCPU)
}

func RandMemoryQuantity() uint64 {
	return RandRangeUint64(
		dtypes.GetValidationConfig().MinUnitMemory,
		dtypes.GetValidationConfig().MaxUnitMemory)
}

func RandStorageQuantity() uint64 {
	return RandRangeUint64(
		dtypes.GetValidationConfig().MinUnitStorage,
		dtypes.GetValidationConfig().MaxUnitStorage)
}

// Resources produces an attribute list for populating a Group's
// 'Resources' fields.
func Resources(t testing.TB) []dtypes.Resource {
	t.Helper()
	count := rand.Intn(10) + 1

	vals := make([]dtypes.Resource, 0, count)
	for i := 0; i < count; i++ {
		coin := sdk.NewCoin(CoinDenom, sdk.NewInt(rand.Int63n(9999)+1))
		res := dtypes.Resource{
			Resources: types.ResourceUnits{
				CPU: &types.CPU{
					Units: types.NewResourceValue(uint64(dtypes.GetValidationConfig().MinUnitCPU)),
				},
				Memory: &types.Memory{
					Quantity: types.NewResourceValue(dtypes.GetValidationConfig().MinUnitMemory),
				},
				Storage: types.Volumes{
					types.Storage{
						Quantity: types.NewResourceValue(dtypes.GetValidationConfig().MinUnitStorage),
					},
				},
			},
			Count: 1,
			Price: coin,
		}
		vals = append(vals, res)
	}
	return vals
}
