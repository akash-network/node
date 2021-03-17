package bidengine

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"os/exec"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/types/unit"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/shopspring/decimal"
)

type BidPricingStrategy interface {
	CalculatePrice(ctx context.Context, gspec *dtypes.GroupSpec) (sdk.DecCoin, error)
}

const denom = "uakt"

var errAllScalesZero = errors.New("At least one bid price must be a non-zero number")

type scalePricing struct {
	cpuScale      decimal.Decimal
	memoryScale   decimal.Decimal
	storageScale  decimal.Decimal
	endpointScale decimal.Decimal
}

func MakeScalePricing(
	cpuScale decimal.Decimal,
	memoryScale decimal.Decimal,
	storageScale decimal.Decimal,
	endpointScale decimal.Decimal) (BidPricingStrategy, error) {

	if cpuScale.IsZero() && memoryScale.IsZero() && storageScale.IsZero() && endpointScale.IsZero() {
		return nil, errAllScalesZero
	}

	result := scalePricing{
		cpuScale:      cpuScale,
		memoryScale:   memoryScale,
		storageScale:  storageScale,
		endpointScale: endpointScale,
	}

	return result, nil
}

var ErrBidQuantityInvalid = errors.New("A bid quantity is invalid")
var ErrBidZero = errors.New("A bid of zero was produced")

func ceilBigRatToBigInt(v *big.Rat) *big.Int {
	numerator := v.Num()
	denom := v.Denom()

	result := big.NewInt(0).Div(numerator, denom)
	if !v.IsInt() {
		result.Add(result, big.NewInt(1))
	}

	return result
}

func (fp scalePricing) CalculatePrice(ctx context.Context, gspec *dtypes.GroupSpec) (sdk.DecCoin, error) {
	// Use unlimited precision math here.
	// Otherwise a correctly crafted order could create a cost of '1' given
	// a possible configuration
	cpuTotal := decimal.NewFromInt(0)
	memoryTotal := decimal.NewFromInt(0)
	storageTotal := decimal.NewFromInt(0)
	endpointTotal := decimal.NewFromInt(0)

	// iterate over everything & sum it up
	for _, group := range gspec.Resources {
		groupCount := decimal.NewFromInt(int64(group.Count)) // Expand uint32 to int64

		cpuQuantity := decimal.NewFromBigInt(group.Resources.CPU.Units.Val.BigInt(), 0)
		cpuQuantity = cpuQuantity.Mul(groupCount)
		cpuTotal = cpuTotal.Add(cpuQuantity)

		memoryQuantity := decimal.NewFromBigInt(group.Resources.Memory.Quantity.Val.BigInt(), 0)
		memoryQuantity = memoryQuantity.Mul(groupCount)
		memoryTotal = memoryTotal.Add(memoryQuantity)

		storageQuantity := decimal.NewFromBigInt(group.Resources.Storage.Quantity.Val.BigInt(), 0)
		storageQuantity = storageQuantity.Mul(groupCount)
		storageTotal = storageTotal.Add(storageQuantity)

		endpointQuantity := decimal.NewFromInt(int64(len(group.Resources.Endpoints)))
		endpointTotal = endpointTotal.Add(endpointQuantity)
	}

	cpuTotal = cpuTotal.Mul(fp.cpuScale)

	mebibytes := decimal.NewFromInt(unit.Mi)

	memoryTotal = memoryTotal.Div(mebibytes)
	memoryTotal = memoryTotal.Mul(fp.memoryScale)

	storageTotal = storageTotal.Div(mebibytes)
	storageTotal = storageTotal.Mul(fp.storageScale)

	endpointTotal = endpointTotal.Mul(fp.endpointScale)

	decimal.NewFromInt(math.MaxInt64)
	// Each quantity must be non negative
	// and fit into an Int64
	if cpuTotal.IsNegative() ||
		memoryTotal.IsNegative() ||
		storageTotal.IsNegative() ||
		endpointTotal.IsNegative() {
		return sdk.DecCoin{}, ErrBidQuantityInvalid
	}

	totalCost := cpuTotal
	totalCost = totalCost.Add(memoryTotal)
	totalCost = totalCost.Add(storageTotal)
	totalCost = totalCost.Add(endpointTotal)

	if totalCost.IsNegative() {
		return sdk.DecCoin{}, ErrBidQuantityInvalid
	}

	if totalCost.IsZero() {
		// Return an error indicating we can't bid with a cost of zero
		return sdk.DecCoin{}, ErrBidZero
	}

	costDec, err := sdk.NewDecFromStr(totalCost.String())
	if err != nil {
		return sdk.DecCoin{}, err
	}

	if !costDec.LTE(sdk.MaxSortableDec) {
		return sdk.DecCoin{}, ErrBidQuantityInvalid
	}

	return sdk.NewDecCoinFromDec(denom, costDec), nil
}

type randomRangePricing int

func MakeRandomRangePricing() (BidPricingStrategy, error) {
	return randomRangePricing(0), nil
}

func (randomRangePricing) CalculatePrice(ctx context.Context, gspec *dtypes.GroupSpec) (sdk.DecCoin, error) {
	min, max := calculatePriceRange(gspec)
	if min.IsEqual(max) {
		return max, nil
	}

	const scale = 10000

	delta := max.Amount.Sub(min.Amount).Mul(sdk.NewDec(scale))

	minbid := delta.TruncateInt64()
	if minbid < 1 {
		minbid = 1
	}
	val, err := rand.Int(rand.Reader, big.NewInt(minbid))
	if err != nil {
		return sdk.DecCoin{}, err
	}

	scaledValue := sdk.NewDecFromBigInt(val).QuoInt64(scale).QuoInt64(100)
	amount := min.Amount.Add(scaledValue)
	return sdk.NewDecCoinFromDec(min.Denom, amount), nil
}

func calculatePriceRange(gspec *dtypes.GroupSpec) (sdk.DecCoin, sdk.DecCoin) {
	// memory-based pricing:
	//   min: requested memory * configured min price per Gi
	//   max: requested memory * configured max price per Gi

	// assumption: group.Count > 0
	// assumption: all same denom (returned by gspec.Price())
	// assumption: gspec.Price() > 0

	mem := sdk.NewInt(0)

	for _, group := range gspec.Resources {
		mem = mem.Add(
			sdk.NewIntFromUint64(group.Resources.Memory.Quantity.Value()).
				MulRaw(int64(group.Count)))
	}

	rmax := gspec.Price()

	const minGroupMemPrice = int64(50)
	const maxGroupMemPrice = int64(1048576)

	cmin := sdk.NewDecFromInt(mem.MulRaw(
		minGroupMemPrice).
		Quo(sdk.NewInt(unit.Gi)))

	cmax := sdk.NewDecFromInt(mem.MulRaw(
		maxGroupMemPrice).
		Quo(sdk.NewInt(unit.Gi)))

	if cmax.GT(rmax.Amount) {
		cmax = rmax.Amount
	}

	if cmin.IsZero() {
		cmin = sdk.NewDec(1)
	}

	if cmax.IsZero() {
		cmax = sdk.NewDec(1)
	}

	return sdk.NewDecCoinFromDec(rmax.Denom, cmin), sdk.NewDecCoinFromDec(rmax.Denom, cmax)
}

type shellScriptPricing struct {
	path         string
	processLimit chan int
	runtimeLimit time.Duration
}

var errPathEmpty = errors.New("script path cannot be the empty string")
var errProcessLimitZero = errors.New("process limit must be greater than zero")
var errProcessRuntimeLimitZero = errors.New("process runtime limit must be greater than zero")

func MakeShellScriptPricing(path string, processLimit uint, runtimeLimit time.Duration) (BidPricingStrategy, error) {
	if len(path) == 0 {
		return nil, errPathEmpty
	}
	if processLimit == 0 {
		return nil, errProcessLimitZero
	}
	if runtimeLimit == 0 {
		return nil, errProcessRuntimeLimitZero
	}

	result := shellScriptPricing{
		path:         path,
		processLimit: make(chan int, processLimit),
		runtimeLimit: runtimeLimit,
	}

	// Use the channel as a semaphore to limit the number of processes created for computing bid processes
	// Most platforms put a limit on the number of processes a user can open. Even if the limit is high
	// it isn't a good idea to open thuosands of processes.
	for i := uint(0); i != processLimit; i++ {
		result.processLimit <- 0
	}

	return result, nil
}

type dataForScriptElement struct {
	Memory           uint64 `json:"memory"`
	CPU              uint64 `json:"cpu"`
	Storage          uint64 `json:"storage"`
	Count            uint32 `json:"count"`
	EndpointQuantity int    `json:"endpoint_quantity"`
}

func (ssp shellScriptPricing) CalculatePrice(ctx context.Context, gspec *dtypes.GroupSpec) (sdk.DecCoin, error) {
	buf := &bytes.Buffer{}

	dataForScript := make([]dataForScriptElement, len(gspec.Resources))

	// iterate over everything & sum it up
	for i, group := range gspec.Resources {
		groupCount := group.Count
		cpuQuantity := group.Resources.CPU.Units.Val.Uint64()
		memoryQuantity := group.Resources.Memory.Quantity.Value()
		storageQuantity := group.Resources.Storage.Quantity.Val.Uint64()
		endpointQuantity := len(group.Resources.Endpoints)

		dataForScript[i] = dataForScriptElement{
			CPU:              cpuQuantity,
			Memory:           memoryQuantity,
			Storage:          storageQuantity,
			Count:            groupCount,
			EndpointQuantity: endpointQuantity,
		}
	}

	encoder := json.NewEncoder(buf)
	err := encoder.Encode(dataForScript)
	if err != nil {
		return sdk.DecCoin{}, err
	}

	// Take 1 from the channel
	<-ssp.processLimit
	defer func() {
		// Always return it when this function is complete
		ssp.processLimit <- 0
	}()

	processCtx, cancel := context.WithTimeout(ctx, ssp.runtimeLimit)
	defer cancel()
	cmd := exec.CommandContext(processCtx, ssp.path) //nolint:gosec
	cmd.Stdin = buf
	outputBuf := &bytes.Buffer{}
	cmd.Stdout = outputBuf

	err = cmd.Run()
	if ctxErr := processCtx.Err(); ctxErr != nil {
		return sdk.DecCoin{}, ctxErr
	}
	if err != nil {
		return sdk.DecCoin{}, err
	}

	// Decode the result
	decoder := json.NewDecoder(outputBuf)
	decoder.UseNumber()

	var priceNumber json.Number
	err = decoder.Decode(&priceNumber)
	if err != nil {
		return sdk.DecCoin{}, err
	}

	price, err := sdk.NewDecFromStr(priceNumber.String())
	if err != nil {
		return sdk.DecCoin{}, ErrBidQuantityInvalid
	}

	if price.IsZero() {
		return sdk.DecCoin{}, ErrBidZero
	}

	if price.IsNegative() {
		return sdk.DecCoin{}, ErrBidQuantityInvalid
	}

	if !price.LTE(sdk.MaxSortableDec) {
		return sdk.DecCoin{}, ErrBidQuantityInvalid
	}

	return sdk.NewDecCoinFromDec(denom, price), nil
}
