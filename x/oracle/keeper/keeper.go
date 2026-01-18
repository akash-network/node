package keeper

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/store"
	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	types "pkg.akt.dev/go/node/oracle/v1"
	"pkg.akt.dev/go/sdkutil"
)

type SetParamsHook func(sdk.Context, types.Params)

type Keeper interface {
	StoreKey() storetypes.StoreKey
	Codec() codec.BinaryCodec
	GetAuthority() string
	NewQuerier() Querier
	BeginBlocker(ctx context.Context) error
	EndBlocker(ctx context.Context) error
	GetParams(sdk.Context) (types.Params, error)
	SetParams(sdk.Context, types.Params) error

	AddPriceEntry(sdk.Context, sdk.Address, types.DataID, types.PriceDataState) error
	GetAggregatedPrice(ctx sdk.Context, denom string) (sdkmath.LegacyDec, error)
	SetAggregatedPrice(sdk.Context, types.DataID, types.AggregatedPrice) error
	SetPriceHealth(sdk.Context, types.DataID, types.PriceHealth) error
}

// Keeper of the deployment store
type keeper struct {
	cdc  codec.BinaryCodec
	skey *storetypes.KVStoreKey
	ssvc store.KVStoreService
	// The address capable of executing an MsgUpdateParams message.
	// This should be the x/gov module account.
	authority             string
	priceWriteAuthorities []string

	Schema collections.Schema
	Params collections.Item[types.Params]

	collections.Sequence
	latestPrices     collections.Map[types.PriceDataID, int64]
	aggregatedPrices collections.Map[types.DataID, types.AggregatedPrice]
	pricesHealth     collections.Map[types.DataID, types.PriceHealth]
	prices           collections.Map[types.PriceDataRecordID, types.PriceDataState]
	sourceSequence   collections.Sequence
	sourceID         collections.Map[string, uint32]
	hooks            struct {
		onSetParams []SetParamsHook
	}
}

// NewKeeper creates and returns an instance of take keeper
func NewKeeper(cdc codec.BinaryCodec, skey *storetypes.KVStoreKey, authority string) Keeper {
	ssvc := runtime.NewKVStoreService(skey)
	sb := collections.NewSchemaBuilder(ssvc)

	k := &keeper{
		cdc:              cdc,
		skey:             skey,
		ssvc:             runtime.NewKVStoreService(skey),
		authority:        authority,
		Params:           collections.NewItem(sb, ParamsKey, "params", codec.CollValue[types.Params](cdc)),
		latestPrices:     collections.NewMap(sb, LatestPricesPrefix, "latest_prices", PriceDataIDKey, collections.Int64Value),
		aggregatedPrices: collections.NewMap(sb, AggregatedPricesPrefix, "aggregated_prices", DataIDKey, codec.CollValue[types.AggregatedPrice](cdc)),
		pricesHealth:     collections.NewMap(sb, PricesHealthPrefix, "prices_health", DataIDKey, codec.CollValue[types.PriceHealth](cdc)),
		prices:           collections.NewMap(sb, PricesPrefix, "prices", PriceDataRecordIDKey, codec.CollValue[types.PriceDataState](cdc)),
		sourceSequence:   collections.NewSequence(sb, SourcesSeqPrefix, "sources_sequence"),
		sourceID:         collections.NewMap(sb, SourcesIDPrefix, "sources_id", collections.StringKey, collections.Uint32Value),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}

	k.Schema = schema

	return k
}

// Codec returns keeper codec
func (k *keeper) Codec() codec.BinaryCodec {
	return k.cdc
}

func (k *keeper) StoreKey() storetypes.StoreKey {
	return k.skey
}

func (k *keeper) Logger(sctx sdk.Context) log.Logger {
	return sctx.Logger().With("module", "x/"+types.ModuleName)
}

func (k *keeper) NewQuerier() Querier {
	return Querier{k}
}

// GetAuthority returns the x/mint module's authority.
func (k *keeper) GetAuthority() string {
	return k.authority
}

// AddPriceEntry adds a price from a specific source (e.g., smart contract)
// This implements multi-source price validation with deviation checks
func (k *keeper) AddPriceEntry(ctx sdk.Context, source sdk.Address, id types.DataID, price types.PriceDataState) error {
	sourceID, authorized := k.getAuthorizedSource(ctx, source.String())
	if !authorized {
		return errorsmod.Wrapf(
			sdkerrors.ErrUnauthorized,
			"source %s is not authorized oracle provider",
			source.String(),
		)
	}

	if id.Denom != sdkutil.DenomAkt {
		return errorsmod.Wrapf(
			sdkerrors.ErrInvalidRequest,
			"unsupported denom %s", id.Denom,
		)
	}

	if id.BaseDenom != sdkutil.DenomUSD {
		return errorsmod.Wrapf(
			sdkerrors.ErrInvalidRequest,
			"unsupported base denom %s", id.BaseDenom,
		)
	}

	if !price.Price.IsPositive() {
		return errorsmod.Wrap(
			sdkerrors.ErrInvalidRequest,
			"price must be positive",
		)
	}

	if price.Timestamp.After(ctx.BlockTime()) {
		return errorsmod.Wrap(
			sdkerrors.ErrInvalidRequest,
			"price timestamp is from future",
		)
	}

	latestHeight, err := k.latestPrices.Get(ctx, types.PriceDataID{
		Source:    sourceID,
		Denom:     id.Denom,
		BaseDenom: id.BaseDenom,
	})
	if err != nil && !errors.Is(err, collections.ErrNotFound) {
		return err
	}

	// timestamp of new datapoint must be newer than existing
	// if this is the first data point, then it should be not older than 2 blocks back
	if err == nil {
		latest, err := k.prices.Get(ctx, types.PriceDataRecordID{
			Source:    sourceID,
			Denom:     id.Denom,
			BaseDenom: id.BaseDenom,
			Height:    latestHeight,
		})
		// a record must exist at this point; any error means something went horribly wrong
		if err != nil {
			return err
		}
		if price.Timestamp.Before(latest.Timestamp) {
			return errorsmod.Wrap(
				sdkerrors.ErrInvalidRequest,
				"price timestamp is older than existing record",
			)
		}
	} else if ctx.BlockTime().Sub(price.Timestamp) > time.Second*12 { // fixme should be parameter
		return errorsmod.Wrap(
			sdkerrors.ErrInvalidRequest,
			"price timestamp is too old",
		)
	}

	recordID := types.PriceDataRecordID{
		Source:    sourceID,
		Denom:     id.Denom,
		BaseDenom: id.BaseDenom,
		Height:    ctx.BlockHeight(),
	}

	err = k.prices.Set(ctx, recordID, price)
	if err != nil {
		return err
	}

	err = k.latestPrices.Set(ctx, types.PriceDataID{
		Source:    sourceID,
		Denom:     id.Denom,
		BaseDenom: id.BaseDenom,
	}, recordID.Height)
	if err != nil {
		return err
	}

	// todo price aggregation and health check is done within end blocker
	// it should be updated here as well

	err = ctx.EventManager().EmitTypedEvent(
		&types.EventPriceData{
			Source: source.String(),
			Id:     id,
			Data:   price,
		},
	)

	if err != nil {
		return err
	}

	return nil
}

func (k *keeper) GetAggregatedPrice(ctx sdk.Context, denom string) (sdkmath.LegacyDec, error) {
	var res sdkmath.LegacyDec

	// Normalize denom: convert micro denoms to base denoms for oracle lookups
	// Oracle stores prices for base denoms (akt, usdc, etc.) not micro denoms
	normalizedDenom := denom
	if denom == sdkutil.DenomUakt {
		normalizedDenom = sdkutil.DenomAkt
	} else if denom == sdkutil.DenomUact {
		normalizedDenom = sdkutil.DenomAct
	}

	// ACT is always pegged to 1USD
	if normalizedDenom == sdkutil.DenomAct {
		return sdkmath.LegacyOneDec(), nil
	}

	id := types.DataID{
		Denom:     normalizedDenom,
		BaseDenom: sdkutil.DenomUSD,
	}

	health, err := k.pricesHealth.Get(ctx, id)
	if err != nil {
		return res, errorsmod.Wrap(types.ErrPriceStalled, err.Error())
	}

	if !health.IsHealthy {
		return res, types.ErrPriceStalled
	}

	price, err := k.aggregatedPrices.Get(ctx, id)
	if err != nil {
		return res, errorsmod.Wrap(types.ErrPriceStalled, err.Error())
	}

	return price.MedianPrice, nil
}

// BeginBlocker checks if prices are being updated and sources do not deviate from each other
// price for requested denom halts if any of the following conditions occur
// - the price have not been updated within UpdatePeriod
// - price deviation between multiple sources is more than TBD
func (k *keeper) BeginBlocker(ctx context.Context) error {
	sctx := sdk.UnwrapSDKContext(ctx)

	// at this stage oracle is testnet only
	// so we panic here to prevent any use on mainnet
	if sctx.ChainID() == "akashnet-2" {
		panic(fmt.Sprint("x/oracle cannot be used on mainnet just yet"))
	}

	return nil
}

// EndBlocker is called at the end of each block to manage snapshots.
// It records periodic snapshots and prunes old ones.
func (k *keeper) EndBlocker(ctx context.Context) error {
	sctx := sdk.UnwrapSDKContext(ctx)

	params, _ := k.GetParams(sctx)

	var rid []types.PriceDataRecordID

	cutoffHeight := sctx.BlockHeight() - params.MaxPriceStalenessBlocks

	_ = k.latestPrices.Walk(sctx, nil, func(key types.PriceDataID, height int64) (bool, error) {
		if height >= cutoffHeight {
			rid = append(rid, types.PriceDataRecordID{
				Source:    key.Source,
				Denom:     key.Denom,
				BaseDenom: key.BaseDenom,
				Height:    height,
			})
		}

		return false, nil
	})

	latestData := make([]types.PriceData, 0, len(rid))

	for _, id := range rid {
		state, _ := k.prices.Get(sctx, id)

		latestData = append(latestData, types.PriceData{
			ID:    id,
			State: state,
		})
	}
	// Aggregate prices from all active sources
	aggregatedPrice, err := k.calculateAggregatedPrices(sctx, latestData)
	if err != nil {
		sctx.Logger().Error(
			"calculate aggregated price",
			"reason", err.Error(),
		)
	}

	health := k.setPriceHealth(sctx, params, aggregatedPrice)

	// If healthy and we have price data, update the final oracle price
	if health.IsHealthy && len(latestData) > 0 {
		id := types.DataID{
			Denom:     latestData[0].ID.Denom,
			BaseDenom: latestData[0].ID.BaseDenom,
		}

		err = k.aggregatedPrices.Set(sctx, id, aggregatedPrice)
		if err != nil {
			sctx.Logger().Error(
				"set aggregated price",
				"reason", err.Error(),
			)
		}
	}

	return nil
}

// isAuthorizedSource checks if an address is authorized to provide oracle data
func (k *keeper) getAuthorizedSource(ctx sdk.Context, source string) (uint32, bool) {
	params, err := k.GetParams(ctx)
	if err != nil {
		return 0, false
	}

	for _, record := range params.Sources {
		if record == source {
			// load source ID

			id, err := k.sourceID.Get(ctx, source)
			if err != nil {
				return id, false
			}

			return id, true
		}
	}

	return 0, false
}

// getTWAPHistory retrieves TWAP history for a source within a block range
func (k *keeper) getTWAPHistory(ctx sdk.Context, source uint32, denom string, startBlock int64, endBlock int64) []types.PriceData {
	var res []types.PriceData

	start := types.PriceDataRecordID{
		Source:    source,
		Denom:     denom,
		BaseDenom: sdkutil.DenomUSD,
		Height:    startBlock,
	}

	end := types.PriceDataRecordID{
		Source:    source,
		Denom:     denom,
		BaseDenom: sdkutil.DenomUSD,
		Height:    endBlock,
	}

	rng := new(collections.Range[types.PriceDataRecordID]).
		StartInclusive(start).
		EndInclusive(end).
		Descending()

	err := k.prices.Walk(ctx, rng, func(key types.PriceDataRecordID, val types.PriceDataState) (stop bool, err error) {
		res = append(res, types.PriceData{
			ID:    key,
			State: val,
		})

		return false, nil
	})
	if err != nil {
		panic(err.Error())
	}

	return res
}

// SetParams sets the x/oracle module parameters.
func (k *keeper) SetParams(ctx sdk.Context, p types.Params) error {
	if err := p.ValidateBasic(); err != nil {
		return err
	}

	if err := k.Params.Set(ctx, p); err != nil {
		return err
	}

	for _, source := range p.Sources {
		exists, err := k.sourceID.Has(ctx, source)
		if err != nil {
			return err
		}

		if !exists {
			n, err := collections.Item[uint64](k.sourceSequence).Get(ctx)
			if err != nil && !errors.Is(err, collections.ErrNotFound) {
				return err
			}
			// If sequence doesn't exist yet, start at 0
			if errors.Is(err, collections.ErrNotFound) {
				n = 0
			}

			n += 1
			err = k.sourceSequence.Set(ctx, n)
			if err != nil {
				return err
			}

			// todo ideally we check uint32 overflow
			// tho it's going to take a long while to set uint32 max of oracle sources
			err = k.sourceID.Set(ctx, source, uint32(n))
			if err != nil {
				return err
			}
		}
	}
	// call hooks
	for _, hook := range k.hooks.onSetParams {
		hook(ctx, p)
	}

	return nil
}

// GetParams returns the current x/oracle module parameters.
func (k *keeper) GetParams(ctx sdk.Context) (types.Params, error) {
	return k.Params.Get(ctx)
}

// SetAggregatedPrice sets the aggregated price for a denom (for testing)
func (k *keeper) SetAggregatedPrice(ctx sdk.Context, id types.DataID, price types.AggregatedPrice) error {
	return k.aggregatedPrices.Set(ctx, id, price)
}

// SetPriceHealth sets the price health for a denom (for testing)
func (k *keeper) SetPriceHealth(ctx sdk.Context, id types.DataID, health types.PriceHealth) error {
	return k.pricesHealth.Set(ctx, id, health)
}

func (k *keeper) AddOnSetParamsHook(hook SetParamsHook) Keeper {
	k.hooks.onSetParams = append(k.hooks.onSetParams, hook)

	return k
}

// calculateAggregatedPrices aggregates prices from all active sources for a denom
func (k *keeper) calculateAggregatedPrices(ctx sdk.Context, latestData []types.PriceData) (types.AggregatedPrice, error) {
	params, err := k.GetParams(ctx)
	if err != nil {
		return types.AggregatedPrice{}, err
	}

	if len(latestData) == 0 {
		return types.AggregatedPrice{}, errorsmod.Wrap(
			types.ErrPriceStalled,
			"all price sources are stale",
		)
	}

	// Calculate TWAP for each source
	var twaps []sdkmath.LegacyDec //nolint:prealloc
	for _, source := range latestData {
		twap, err := k.calculateTWAPBySource(ctx, source.ID.Source, source.ID.Denom, params.TwapWindow)
		if err != nil {
			ctx.Logger().Error(
				"failed to calculate TWAP for source",
				"source", source.ID.Source,
				"error", err.Error(),
			)
			continue
		}
		twaps = append(twaps, twap)
	}

	if len(twaps) == 0 {
		return types.AggregatedPrice{}, errorsmod.Wrap(
			sdkerrors.ErrInvalidRequest,
			"no valid TWAP calculations",
		)
	}

	// Calculate aggregate TWAP (average of all source TWAPs)
	totalTWAP := sdkmath.LegacyZeroDec()
	for _, twap := range twaps {
		totalTWAP = totalTWAP.Add(twap)
	}
	aggregateTWAP := totalTWAP.Quo(sdkmath.LegacyNewDec(int64(len(twaps))))

	// Calculate median
	medianPrice := calculateMedian(latestData)

	// Calculate min/max
	minPrice := latestData[0].State.Price
	maxPrice := latestData[0].State.Price
	for _, rec := range latestData {
		if rec.State.Price.LT(minPrice) {
			minPrice = rec.State.Price
		}
		if rec.State.Price.GT(maxPrice) {
			maxPrice = rec.State.Price
		}
	}

	// Calculate deviation in basis points
	deviationBps := calculateDeviationBps(minPrice, maxPrice)

	return types.AggregatedPrice{
		Denom:        latestData[0].ID.Denom,
		TWAP:         aggregateTWAP,
		MedianPrice:  medianPrice,
		MinPrice:     minPrice,
		MaxPrice:     maxPrice,
		Timestamp:    ctx.BlockTime(),
		NumSources:   uint32(len(latestData)),
		DeviationBps: deviationBps,
	}, nil
}

// calculateTWABySource calculates TWAP for a specific source over the window
func (k *keeper) calculateTWAPBySource(ctx sdk.Context, source uint32, denom string, windowBlocks int64) (sdkmath.LegacyDec, error) {
	currentHeight := ctx.BlockHeight()
	startHeight := currentHeight - windowBlocks

	// Get historical data points for this source within the window
	dataPoints := k.getTWAPHistory(ctx, source, denom, startHeight, currentHeight)

	if len(dataPoints) == 0 {
		// No historical data, use current price
		return sdkmath.LegacyZeroDec(), errorsmod.Wrap(
			sdkerrors.ErrNotFound,
			"no price data for requested source",
		)
	}

	// Calculate time-weighted average
	weightedSum := sdkmath.LegacyZeroDec()
	totalWeight := int64(0)

	for i := 0; i < len(dataPoints); i++ {
		current := dataPoints[i]

		// Calculate time weight (duration until next point or current time)
		var timeWeight int64
		if i < len(dataPoints)-1 {
			timeWeight = dataPoints[i+1].ID.Height - current.ID.Height
		} else {
			timeWeight = currentHeight - current.ID.Height
		}

		// Add weighted price
		weightedSum = weightedSum.Add(current.State.Price.Mul(sdkmath.LegacyNewDec(timeWeight)))
		totalWeight += timeWeight
	}

	if totalWeight == 0 {
		return sdkmath.LegacyZeroDec(), types.ErrTWAPZeroWeight
	}

	twap := weightedSum.Quo(sdkmath.LegacyNewDec(totalWeight))

	return twap, nil
}

func (k *keeper) getAggregatedPrice(ctx sdk.Context, denom string) (types.AggregatedPrice, error) {
	return k.aggregatedPrices.Get(ctx, types.DataID{
		Denom:     denom,
		BaseDenom: sdkutil.DenomUSD,
	})
}

// CheckPriceHealth checks if the aggregated price meets health requirements
func (k *keeper) setPriceHealth(ctx sdk.Context, params types.Params, aggregatedPrice types.AggregatedPrice) types.PriceHealth {
	health := types.PriceHealth{
		Denom: aggregatedPrice.Denom,
	}

	// Check 1: Minimum number of sources
	if aggregatedPrice.NumSources < params.MinPriceSources {
		health.FailureReason = append(health.FailureReason, fmt.Sprintf(
			"insufficient price sources: %d < %d",
			aggregatedPrice.NumSources,
			params.MinPriceSources,
		))
	}
	health.HasMinSources = true

	// Check 2: Deviation within acceptable range
	if aggregatedPrice.DeviationBps > params.MaxPriceDeviationBps {
		health.FailureReason = append(health.FailureReason, fmt.Sprintf(
			"price deviation too high: %dbps > %dbps",
			aggregatedPrice.DeviationBps,
			params.MaxPriceDeviationBps,
		))
	}
	health.DeviationOk = true

	// Check 3: All sources are fresh
	allFresh := true
	cutoffHeight := ctx.BlockHeight() - params.MaxPriceStalenessBlocks
	err := k.latestPrices.Walk(ctx, nil, func(_ types.PriceDataID, value int64) (bool, error) {
		allFresh = value >= cutoffHeight

		return !allFresh, nil
	})

	if err != nil {
		allFresh = false
	}

	if !allFresh {
		health.FailureReason = append(health.FailureReason, "one or more price sources are stale")
	}

	health.AllSourcesFresh = true
	health.IsHealthy = true

	err = k.pricesHealth.Set(ctx, types.DataID{Denom: health.Denom, BaseDenom: sdkutil.DenomUSD}, health)
	// if there is an error when storing price health, something went horribly wrong
	if err != nil {
		panic(err)
	}

	return health
}

// Helper functions
func calculateMedian(prices []types.PriceData) sdkmath.LegacyDec {
	if len(prices) == 0 {
		return sdkmath.LegacyZeroDec()
	}

	// Sort prices
	sortedPrices := make([]types.PriceData, len(prices))
	copy(sortedPrices, prices)
	sort.Slice(sortedPrices, func(i, j int) bool {
		return sortedPrices[i].State.Price.LT(sortedPrices[j].State.Price)
	})

	mid := len(sortedPrices) / 2
	if len(sortedPrices)%2 == 0 {
		// Even: average of two middle values
		return sortedPrices[mid-1].State.Price.Add(sortedPrices[mid].State.Price).Quo(sdkmath.LegacyNewDec(2))
	}
	// Odd: middle value
	return sortedPrices[mid].State.Price
}

func calculateDeviationBps(minPrice, maxPrice sdkmath.LegacyDec) uint64 {
	if minPrice.IsZero() {
		return 0
	}

	diff := maxPrice.Sub(minPrice)
	deviation := diff.Mul(sdkmath.LegacyNewDec(10000)).Quo(minPrice)

	return deviation.TruncateInt().Abs().Uint64()
}
