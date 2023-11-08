package testutil

import (
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/tendermint/tendermint/libs/rand"

	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	types "github.com/akash-network/akash-api/go/node/types/v1beta3"

	// ensure sdkutil.init() to seal SDK config for the tests
	_ "github.com/akash-network/akash-api/go/sdkutil"
)

// CoinDenom provides ability to create coins in test functions and
// pass them into testutil functionality.
const (
	CoinDenom = "uakt"
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
		dtypes.GetValidationConfig().Unit.Min.CPU,
		dtypes.GetValidationConfig().Unit.Max.CPU)
}

func RandGPUUnits() uint {
	return RandRangeUint(
		dtypes.GetValidationConfig().Unit.Min.GPU,
		dtypes.GetValidationConfig().Unit.Max.GPU)
}

func RandMemoryQuantity() uint64 {
	return RandRangeUint64(
		dtypes.GetValidationConfig().Unit.Min.Memory,
		dtypes.GetValidationConfig().Unit.Max.Memory)
}

func RandStorageQuantity() uint64 {
	return RandRangeUint64(
		dtypes.GetValidationConfig().Unit.Min.Storage,
		dtypes.GetValidationConfig().Unit.Max.Storage)
}

// Resources produces an attribute list for populating a Group's
// 'Resources' fields.
func Resources(t testing.TB) []dtypes.ResourceUnit {
	t.Helper()
	count := rand.Intn(10) + 1

	vals := make(dtypes.ResourceUnits, 0, count)
	for i := 0; i < count; i++ {
		coin := sdk.NewDecCoin(CoinDenom, sdk.NewInt(rand.Int63n(9999)+1))
		res := dtypes.ResourceUnit{
			Resources: types.Resources{
				ID: uint32(i) + 1,
				CPU: &types.CPU{
					Units: types.NewResourceValue(uint64(dtypes.GetValidationConfig().Unit.Min.CPU)),
				},
				GPU: &types.GPU{
					Units: types.NewResourceValue(uint64(dtypes.GetValidationConfig().Unit.Min.GPU)),
				},
				Memory: &types.Memory{
					Quantity: types.NewResourceValue(dtypes.GetValidationConfig().Unit.Min.Memory),
				},
				Storage: types.Volumes{
					types.Storage{
						Quantity: types.NewResourceValue(dtypes.GetValidationConfig().Unit.Min.Storage),
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
