package oracle

import (
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	oraclev1 "pkg.akt.dev/go/node/oracle/v1"
	sdkutil "pkg.akt.dev/go/sdkutil"

	oraclekeeper "pkg.akt.dev/node/v2/x/oracle/keeper"
)

// PriceFeeder is a test utility that manages oracle price feeds for testing
type PriceFeeder struct {
	keeper        oraclekeeper.Keeper
	sourceAddress sdk.AccAddress
	prices        map[string]sdkmath.LegacyDec // denom -> price in USD
}

// NewPriceFeeder creates a new price feeder for testing
// It sets up the oracle with a test source and initializes default prices
func NewPriceFeeder(keeper oraclekeeper.Keeper, sourceAddress sdk.AccAddress) *PriceFeeder {
	pf := &PriceFeeder{
		keeper:        keeper,
		sourceAddress: sourceAddress,
		prices:        make(map[string]sdkmath.LegacyDec),
	}

	// Set default prices
	pf.prices[sdkutil.DenomAkt] = sdkmath.LegacyMustNewDecFromStr("3.0") // $3.00 per AKT

	return pf
}

// SetupPriceFeeder initializes the oracle module with a test price source
// and registers the source. This should be called during test setup.
func SetupPriceFeeder(ctx sdk.Context, keeper oraclekeeper.Keeper, t ...interface{}) (*PriceFeeder, error) {
	// Create a test oracle source address
	// Generate a deterministic address for tests
	sourceAddress := sdk.AccAddress([]byte("oracle_test_source_address_0001"))

	// Set oracle params with authorized source (source ID will be auto-assigned)
	params := oraclev1.Params{
		Sources:                 []string{sourceAddress.String()},
		MinPriceSources:         1, // Only require 1 source for tests
		MaxPriceStalenessBlocks: 1000,
		TwapWindow:              10,
		MaxPriceDeviationBps:    1000, // 10% max deviation (1000 basis points)
	}

	if err := keeper.SetParams(ctx, params); err != nil {
		return nil, err
	}

	pf := NewPriceFeeder(keeper, sourceAddress)
	return pf, nil
}

// SetPrice sets a custom price for a denom (in USD)
func (pf *PriceFeeder) SetPrice(denom string, priceUSD sdkmath.LegacyDec) {
	pf.prices[denom] = priceUSD
}

// FeedPrice submits a price for a specific denom to the oracle
// This adds the price entry and directly sets aggregated price and health for immediate availability
func (pf *PriceFeeder) FeedPrice(ctx sdk.Context, denom string) error {
	price, exists := pf.prices[denom]
	if !exists {
		price = sdkmath.LegacyOneDec() // default to $1.00 if not set
	}

	// Add price entry
	priceData := oraclev1.PriceDataState{
		Price:     price,
		Timestamp: ctx.BlockTime(),
	}

	dataID := oraclev1.DataID{
		Denom:     denom,
		BaseDenom: sdkutil.DenomUSD,
	}

	if err := pf.keeper.AddPriceEntry(ctx, pf.sourceAddress, dataID, priceData); err != nil {
		return err
	}

	// Directly set aggregated price and health for immediate test availability
	// In production, EndBlocker would calculate these
	aggregatedPrice := oraclev1.AggregatedPrice{
		Denom:        denom,
		TWAP:         price,
		MedianPrice:  price,
		MinPrice:     price,
		MaxPrice:     price,
		NumSources:   1,
		DeviationBps: 0,
	}

	priceHealth := oraclev1.PriceHealth{
		Denom:           denom,
		IsHealthy:       true,
		HasMinSources:   true,
		AllSourcesFresh: true,
		DeviationOk:     true,
		FailureReason:   []string{},
	}

	if err := pf.keeper.SetAggregatedPrice(ctx, dataID, aggregatedPrice); err != nil {
		return err
	}

	if err := pf.keeper.SetPriceHealth(ctx, dataID, priceHealth); err != nil {
		return err
	}

	return nil
}

// FeedPrices feeds all configured prices to the oracle
// This is a convenience method to feed all default prices at once
func (pf *PriceFeeder) FeedPrices(ctx sdk.Context) error {
	for denom := range pf.prices {
		if err := pf.FeedPrice(ctx, denom); err != nil {
			return err
		}
	}
	return nil
}

// UpdatePrice updates an existing price and feeds it to the oracle
func (pf *PriceFeeder) UpdatePrice(ctx sdk.Context, denom string, priceUSD sdkmath.LegacyDec) error {
	pf.SetPrice(denom, priceUSD)
	return pf.FeedPrice(ctx, denom)
}

// AdvanceBlockAndFeed advances the block height and re-feeds prices
// This is useful for testing price staleness and TWAP calculations
func (pf *PriceFeeder) AdvanceBlockAndFeed(ctx sdk.Context, blocks int64) (sdk.Context, error) {
	newCtx := ctx.WithBlockHeight(ctx.BlockHeight() + blocks).
		WithBlockTime(ctx.BlockTime().Add(time.Duration(blocks) * 6 * time.Second))

	if err := pf.FeedPrices(newCtx); err != nil {
		return ctx, err
	}

	return newCtx, nil
}
