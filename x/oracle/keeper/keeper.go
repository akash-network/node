package keeper

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"cosmossdk.io/collections"
	"cosmossdk.io/collections/corecompat"
	"cosmossdk.io/core/store"
	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/gogoproto/proto"

	types "pkg.akt.dev/go/node/oracle/v2"
	"pkg.akt.dev/go/sdkutil"
)

type SetParamsHook func(sdk.Context, types.Params)

type Keeper interface {
	Schema() collections.Schema
	StoreKey() storetypes.StoreKey
	Codec() codec.BinaryCodec
	GetAuthority() string
	NewQuerier() Querier
	BeginBlocker(ctx context.Context) error
	EndBlocker(ctx context.Context) error
	GetParams(sdk.Context) (types.Params, error)
	SetParams(sdk.Context, types.Params) error

	AddPriceEntry(ctx sdk.Context, source sdk.Address, id types.DataID, price sdkmath.LegacyDec, timestamp time.Time) error
	GetAggregatedPrice(ctx sdk.Context, denom string) (sdkmath.LegacyDec, error)
	SetAggregatedPrice(sdk.Context, types.DataID, types.AggregatedPrice) error
	SetPriceHealth(sdk.Context, types.DataID, types.PriceHealth) error

	InitGenesis(ctx sdk.Context, data *types.GenesisState)
	ExportGenesis(ctx sdk.Context) *types.GenesisState
}

// Keeper of the deployment store
type keeper struct {
	cdc  codec.BinaryCodec
	skey *storetypes.KVStoreKey
	ssvc store.KVStoreService
	tsvc store.TransientStoreService
	// The address capable of executing an MsgUpdateParams message.
	// This should be the x/gov module account.
	authority             string
	priceWriteAuthorities []string

	schema  collections.Schema
	tschema collections.Schema
	Params  collections.Item[types.Params]

	collections.Sequence

	// latestPrices are records on when each price pair was last updated
	latestPriceID    collections.Map[types.PriceDataID, types.PriceLatestDataState]
	aggregatedPrices collections.Map[types.DataID, types.AggregatedPrice]
	pricesHealth     collections.Map[types.DataID, types.PriceHealth]
	prices           collections.Map[types.PriceDataRecordID, types.PriceDataState]
	pricesSequence   collections.Map[types.DataID, uint64]
	sourceSequence   collections.Sequence
	sourceID         collections.Map[string, uint32]
	hooks            struct {
		onSetParams []SetParamsHook
	}
}

// NewKeeper creates and returns an instance of take keeper
func NewKeeper(cdc codec.BinaryCodec, skey *storetypes.KVStoreKey, tkey *storetypes.TransientStoreKey, authority string) Keeper {
	ssvc := runtime.NewKVStoreService(skey)
	tsvc := runtime.NewTransientStoreService(tkey)

	sb := collections.NewSchemaBuilder(ssvc)

	tsb := collections.NewSchemaBuilderFromAccessor(func(ctx context.Context) corecompat.KVStore {
		return tsvc.OpenTransientStore(ctx)
	})

	k := &keeper{
		cdc:              cdc,
		skey:             skey,
		ssvc:             ssvc,
		tsvc:             tsvc,
		authority:        authority,
		Params:           collections.NewItem(sb, ParamsKey, "params", codec.CollValue[types.Params](cdc)),
		latestPriceID:    collections.NewMap(sb, LatestPriceIDPrefix, "latest_price_id", PriceDataIDKey, codec.CollValue[types.PriceLatestDataState](cdc)),
		aggregatedPrices: collections.NewMap(sb, AggregatedPricesPrefix, "aggregated_prices", DataIDKey, codec.CollValue[types.AggregatedPrice](cdc)),
		pricesHealth:     collections.NewMap(sb, PricesHealthPrefix, "prices_health", DataIDKey, codec.CollValue[types.PriceHealth](cdc)),
		prices:           collections.NewMap(sb, PricesPrefix, "prices", PriceDataRecordIDKey, codec.CollValue[types.PriceDataState](cdc)),
		sourceSequence:   collections.NewSequence(sb, SourcesSeqPrefix, "sources_sequence"),
		sourceID:         collections.NewMap(sb, SourcesIDPrefix, "sources_id", collections.StringKey, collections.Uint32Value),
		pricesSequence:   collections.NewMap(tsb, PricesSeqPrefix, "prices_sequence", DataIDKey, collections.Uint64Value),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}

	tschema, err := tsb.Build()
	if err != nil {
		panic(err)
	}

	k.schema = schema
	k.tschema = tschema

	return k
}

func (k *keeper) Schema() collections.Schema {
	return k.schema
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

func (k *keeper) loadLatestPriceID(ctx sdk.Context, id types.PriceDataID) (types.PriceDataRecordID, error) {
	ref, err := k.latestPriceID.Get(ctx, id)
	if err != nil {
		return types.PriceDataRecordID{}, err
	}

	return types.PriceDataRecordID{
		Source:    id.Source,
		Denom:     id.Denom,
		BaseDenom: id.BaseDenom,
		Timestamp: ref.Timestamp,
		Sequence:  ref.Sequence,
	}, nil
}

// AddPriceEntry adds a price from a specific source (e.g., smart contract)
// This implements multi-source price validation with deviation checks
func (k *keeper) AddPriceEntry(ctx sdk.Context, source sdk.Address, id types.DataID, price sdkmath.LegacyDec, timestamp time.Time) error {
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

	if !price.IsPositive() {
		return errorsmod.Wrap(
			sdkerrors.ErrInvalidRequest,
			"price must be positive",
		)
	}

	latestID, err := k.loadLatestPriceID(ctx, types.PriceDataID{
		Source:    sourceID,
		Denom:     id.Denom,
		BaseDenom: id.BaseDenom,
	})

	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			return err
		}

		// applies only to very first record.
		// set here reasonable window
		latestID.Timestamp = ctx.BlockTime().Add(-(time.Second * 12))
	}

	if latestID.Timestamp.After(timestamp) {
		return errorsmod.Wrap(
			sdkerrors.ErrInvalidRequest,
			"price timestamp is too old",
		)
	}

	seq, err := k.priceRecordSeq(ctx, types.DataID{
		Denom:     id.Denom,
		BaseDenom: id.BaseDenom,
	})
	if err != nil {
		return err
	}

	recordID := types.PriceDataRecordID{
		Source:    sourceID,
		Denom:     id.Denom,
		BaseDenom: id.BaseDenom,
		Timestamp: timestamp,
		Sequence:  seq,
	}

	err = k.prices.Set(ctx, recordID, types.PriceDataState{Price: price})
	if err != nil {
		return err
	}

	pdID := types.PriceDataID{
		Source:    sourceID,
		Denom:     id.Denom,
		BaseDenom: id.BaseDenom,
	}

	err = k.latestPriceID.Set(ctx, pdID, types.PriceLatestDataState{
		Timestamp: timestamp,
		Sequence:  seq,
	})
	if err != nil {
		return err
	}

	err = ctx.EventManager().EmitTypedEvent(
		&types.EventPriceData{
			Source:    source.String(),
			Id:        id,
			Price:     price,
			Timestamp: timestamp,
		},
	)

	if err != nil {
		return err
	}

	return nil
}

func (k *keeper) priceRecordSeq(sctx sdk.Context, priceID types.DataID) (uint64, error) {
	seq, err := k.pricesSequence.Get(sctx, priceID)
	if err != nil && !errors.Is(err, collections.ErrNotFound) {
		return 0, err
	}

	err = k.pricesSequence.Set(sctx, priceID, seq+1)
	if err != nil {
		return 0, err
	}

	return seq, nil
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

	return price.TWAP, nil
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

// getTWAPHistory retrieves TWAP history for a source within a time range
func (k *keeper) getTWAPHistory(ctx sdk.Context, source uint32, denom string, baseDenom string, startTime time.Time, endTime time.Time) []types.PriceData {
	var res []types.PriceData

	start := types.PriceDataRecordID{
		Source:    source,
		Denom:     denom,
		BaseDenom: baseDenom,
		Timestamp: startTime,
	}

	end := types.PriceDataRecordID{
		Source:    source,
		Denom:     denom,
		BaseDenom: baseDenom,
		Timestamp: endTime,
		Sequence:  math.MaxUint64,
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

	// Determine which sources are being removed so we can clean up their
	// latestPriceID entries. Without this cleanup the EndBlocker would
	// continue to discover (and skip) stale entries every block, and — more
	// critically — the orphaned latestPriceID state prevents the aggregator
	// from recovering after a remove-then-re-add cycle.
	oldParams, oldErr := k.Params.Get(ctx)

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

	// Clean up latestPriceID entries for sources that were removed.
	// This prevents orphaned state from polluting the EndBlocker walk
	// and ensures a re-added source starts fresh.
	if oldErr == nil {
		newSourceSet := make(map[string]struct{}, len(p.Sources))
		for _, s := range p.Sources {
			newSourceSet[s] = struct{}{}
		}

		for _, s := range oldParams.Sources {
			if _, ok := newSourceSet[s]; ok {
				continue
			}

			// Source was removed — resolve its ID and delete latestPriceID entries.
			sID, err := k.sourceID.Get(ctx, s)
			if err != nil {
				// No sourceID mapping means no latestPriceID to clean up.
				continue
			}

			if err := k.removeSourceLatestPriceIDs(ctx, sID); err != nil {
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

// removeSourceLatestPriceIDs deletes all latestPriceID entries for the given
// source ID. This is called when a source is removed from params.Sources so
// that the EndBlocker no longer discovers stale entries
func (k *keeper) removeSourceLatestPriceIDs(ctx sdk.Context, sourceID uint32) error {
	var toDelete []types.PriceDataID

	err := k.latestPriceID.Walk(ctx, nil, func(key types.PriceDataID, _ types.PriceLatestDataState) (bool, error) {
		if key.Source == sourceID {
			toDelete = append(toDelete, key)
		}
		return false, nil
	})
	if err != nil {
		return err
	}

	for _, key := range toDelete {
		if err := k.latestPriceID.Remove(ctx, key); err != nil {
			return err
		}
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

// calculateAggregatedPricesFromHistory computes the aggregated price from
// pre-fetched in-memory data. latestPrices contains the most recent (non-stale)
// price per source; sourcePrices maps sourceID → full history within the TWAP window.
func (k *keeper) calculateAggregatedPricesFromHistory(
	ctx sdk.Context,
	id types.DataID,
	params types.Params,
	latestPrices []types.PriceData,
	sourcePrices map[uint32][]types.PriceData,
) (types.AggregatedPrice, error) {
	aggregated := types.AggregatedPrice{
		Denom: id.Denom,
	}

	if len(latestPrices) == 0 {
		return aggregated, errorsmod.Wrap(
			types.ErrPriceStalled,
			"all price sources are stale",
		)
	}

	now := ctx.BlockTime()

	// Calculate TWAP for each source from pre-fetched history
	var twaps []sdkmath.LegacyDec //nolint:prealloc
	for _, source := range latestPrices {
		dataPoints := sourcePrices[source.ID.Source]
		twap, err := calculateTWAP(now, dataPoints)
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
		return aggregated, errorsmod.Wrap(
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
	medianPrice := calculateMedian(latestPrices)

	// Calculate min/max
	minPrice := latestPrices[0].State.Price
	maxPrice := latestPrices[0].State.Price
	for _, rec := range latestPrices {
		if rec.State.Price.LT(minPrice) {
			minPrice = rec.State.Price
		}
		if rec.State.Price.GT(maxPrice) {
			maxPrice = rec.State.Price
		}
	}

	// Calculate deviation in basis points
	deviationBps := calculateDeviationBps(minPrice, maxPrice)

	aggregated.TWAP = aggregateTWAP
	aggregated.MedianPrice = medianPrice
	aggregated.MinPrice = minPrice
	aggregated.MaxPrice = maxPrice
	aggregated.Timestamp = now
	aggregated.NumSources = uint32(len(latestPrices))
	aggregated.DeviationBps = deviationBps

	return aggregated, nil
}

// calculateTWAP computes time-weighted average price from pre-fetched data points
// (expected in descending timestamp order from getTWAPHistory).
func calculateTWAP(now time.Time, dataPoints []types.PriceData) (sdkmath.LegacyDec, error) {
	if len(dataPoints) == 0 {
		return sdkmath.LegacyZeroDec(), errorsmod.Wrap(
			sdkerrors.ErrNotFound,
			"no price data for requested source",
		)
	}

	weightedSum := sdkmath.LegacyZeroDec()
	totalWeight := int64(0)

	for i := 0; i < len(dataPoints); i++ {
		current := dataPoints[i]

		var timeWeight int64
		if i > 0 {
			// dataPoints are in descending order, so dataPoints[i-1] is newer than current
			timeWeight = dataPoints[i-1].ID.Timestamp.Sub(current.ID.Timestamp).Nanoseconds()
		} else {
			timeWeight = now.Sub(current.ID.Timestamp).Nanoseconds()
		}

		weightedSum = weightedSum.Add(current.State.Price.Mul(sdkmath.LegacyNewDec(timeWeight)))
		totalWeight += timeWeight
	}

	if totalWeight == 0 {
		return sdkmath.LegacyZeroDec(), types.ErrTWAPZeroWeight
	}

	return weightedSum.Quo(sdkmath.LegacyNewDec(totalWeight)), nil
}

func (k *keeper) getAggregatedPrice(ctx sdk.Context, denom string) (types.AggregatedPrice, error) {
	return k.aggregatedPrices.Get(ctx, types.DataID{
		Denom:     denom,
		BaseDenom: sdkutil.DenomUSD,
	})
}

// CheckPriceHealth checks if the aggregated price meets health requirements
func (k *keeper) setPriceHealth(ctx sdk.Context, params types.Params, dataIDs []types.PriceDataRecordID, aggregatedPrice types.AggregatedPrice) types.PriceHealth {
	health := types.PriceHealth{
		Denom:               aggregatedPrice.Denom,
		TotalSources:        uint32(len(dataIDs)),
		TotalHealthySources: aggregatedPrice.NumSources,
	}

	// Check 1: Minimum number of sources
	health.HasMinSources = aggregatedPrice.NumSources >= params.MinPriceSources
	if !health.HasMinSources {
		health.FailureReason = append(health.FailureReason, fmt.Sprintf(
			"insufficient price sources: %d < %d",
			aggregatedPrice.NumSources,
			params.MinPriceSources,
		))
	}

	// Check 2: Deviation within the acceptable range
	health.DeviationOk = aggregatedPrice.DeviationBps <= params.MaxPriceDeviationBps
	if !health.DeviationOk {
		health.FailureReason = append(health.FailureReason, fmt.Sprintf(
			"price deviation too high: %dbps > %dbps",
			aggregatedPrice.DeviationBps,
			params.MaxPriceDeviationBps,
		))
	}

	health.IsHealthy = health.HasMinSources && health.DeviationOk

	id := types.DataID{Denom: health.Denom, BaseDenom: sdkutil.DenomUSD}

	var evt proto.Message

	phealth, err := k.pricesHealth.Get(ctx, id)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			// if there is an error other than not found, something went horribly wrong
			panic(err)
		}

		// this is the very first record so set event to the health status calculated above
		phealth.IsHealthy = !health.IsHealthy
	}

	if health.IsHealthy != phealth.IsHealthy {
		if health.IsHealthy {
			evt = &types.EventPriceRecovered{
				Id:     id,
				Height: ctx.BlockHeight(),
			}
		} else {
			evt = &types.EventPriceStaled{
				Id:         id,
				LastHeight: 0, // 0 is here intentional, at launch there was no point at which price was healthy
			}
		}
	}

	err = k.pricesHealth.Set(ctx, id, health)
	// if there is an error when storing price health, something went horribly wrong
	if err != nil {
		panic(err)
	}

	if evt != nil {
		err = ctx.EventManager().EmitTypedEvent(evt)
		if err != nil {
			ctx.Logger().Error("failed to emit oracle price status change event", "error", err)
		}
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
