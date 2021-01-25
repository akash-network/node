package bidengine

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"math/big"
	"os/exec"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/types/unit"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

type BidPricingStrategy interface {
	calculatePrice(ctx context.Context, gspec *dtypes.GroupSpec) (sdk.Coin, error)
}

const denom = "uakt"

var errAllScalesZero = errors.New("At least one bid price must be a non-zero number")

type scalePricing struct {
	cpuScale      uint64
	memoryScale   uint64
	storageScale  uint64
	endpointScale uint64
}

func MakeScalePricing(
	cpuScale uint64,
	memoryScale uint64,
	storageScale uint64,
	endpointScale uint64) (BidPricingStrategy, error) {

	if cpuScale == 0 && memoryScale == 0 && storageScale == 0 && endpointScale == 0 {
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

func (fp scalePricing) calculatePrice(ctx context.Context, gspec *dtypes.GroupSpec) (sdk.Coin, error) {
	// Use unlimited precision math here.
	// Otherwise a correctly crafted order could create a cost of '1' given
	// a possible configuration
	cpuTotal := big.NewInt(0)
	memoryTotal := big.NewRat(0, 1)
	storageTotal := big.NewRat(0, 1)
	endpointTotal := big.NewInt(0)

	// iterate over everything & sum it up
	for _, group := range gspec.Resources {

		groupCount := big.NewInt(0)
		groupCount.SetUint64(uint64(group.Count)) // Expand uint32 to uint64
		groupCountRat := big.NewRat(0, 1)
		groupCountRat.SetUint64(uint64(group.Count))

		cpuQuantity := big.NewInt(0)
		cpuQuantity.SetUint64(group.Resources.CPU.Units.Val.Uint64())
		cpuQuantity.Mul(cpuQuantity, groupCount)
		cpuTotal.Add(cpuTotal, cpuQuantity)

		memoryQuantity := big.NewRat(0, 1)
		memoryQuantity.SetUint64(group.Resources.Memory.Quantity.Value())
		memoryQuantity.Mul(memoryQuantity, groupCountRat)
		memoryTotal.Add(memoryTotal, memoryQuantity)

		storageQuantity := big.NewRat(0, 1)
		storageQuantity.SetUint64(group.Resources.Storage.Quantity.Val.Uint64())
		storageQuantity.Mul(storageQuantity, groupCountRat)
		storageTotal.Add(storageTotal, storageQuantity)

		endpointQuantity := big.NewInt(0)
		endpointQuantity.SetUint64(uint64(len(group.Resources.Endpoints)))
		endpointTotal.Add(endpointTotal, endpointQuantity)
	}

	scale := big.NewInt(0)
	scale.SetUint64(fp.cpuScale)
	cpuTotal.Mul(cpuTotal, scale)

	mebibytes := big.NewRat(unit.Mi, 1)
	memoryTotal.Quo(memoryTotal, mebibytes)

	scaleRat := big.NewRat(0, 1)
	scaleRat.SetUint64(fp.memoryScale)
	memoryTotal.Mul(memoryTotal, scaleRat)

	storageTotal.Quo(storageTotal, mebibytes)
	scaleRat.SetUint64(fp.storageScale)
	storageTotal.Mul(storageTotal, scaleRat)

	scale.SetUint64(fp.endpointScale)
	endpointTotal.Mul(endpointTotal, scale)

	memoryTotalInt := ceilBigRatToBigInt(memoryTotal)
	storageTotalInt := ceilBigRatToBigInt(storageTotal)

	// Each quantity must be non negative
	// and fit into an Int64
	if cpuTotal.Sign() < 0 || !cpuTotal.IsInt64() ||
		memoryTotal.Sign() < 0 || !memoryTotalInt.IsInt64() ||
		storageTotal.Sign() < 0 || !storageTotalInt.IsInt64() ||
		endpointTotal.Sign() < 0 || !endpointTotal.IsInt64() {
		return sdk.Coin{}, ErrBidQuantityInvalid
	}

	cpuCost := sdk.NewCoin(denom, sdk.NewIntFromBigInt(cpuTotal))
	memoryCost := sdk.NewCoin(denom, sdk.NewIntFromBigInt(memoryTotalInt))
	storageCost := sdk.NewCoin(denom, sdk.NewIntFromBigInt(storageTotalInt))

	// Check for less than or equal to zero
	cost := cpuCost.Add(memoryCost).Add(storageCost)

	if cost.Amount.IsZero() {
		// Return an error indicating we can't bid with a cost of zero
		return sdk.Coin{}, ErrBidZero
	}

	return cost, nil
}

type randomRangePricing int

func MakeRandomRangePricing() (BidPricingStrategy, error) {
	return randomRangePricing(0), nil
}

func (randomRangePricing) calculatePrice(ctx context.Context, gspec *dtypes.GroupSpec) (sdk.Coin, error) {

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

	for _, group := range gspec.Resources {
		mem = mem.Add(
			sdk.NewIntFromUint64(group.Resources.Memory.Quantity.Value()).
				MulRaw(int64(group.Count)))
	}

	rmax := gspec.Price()

	const minGroupMemPrice = int64(50)
	const maxGroupMemPrice = int64(1048576)

	cmin := mem.MulRaw(
		minGroupMemPrice).
		Quo(sdk.NewInt(unit.Gi))

	cmax := mem.MulRaw(
		maxGroupMemPrice).
		Quo(sdk.NewInt(unit.Gi))

	if cmax.GT(rmax.Amount) {
		cmax = rmax.Amount
	}

	if cmin.IsZero() {
		cmin = sdk.NewInt(1)
	}

	if cmax.IsZero() {
		cmax = sdk.NewInt(1)
	}

	return sdk.NewCoin(rmax.Denom, cmin), sdk.NewCoin(rmax.Denom, cmax)
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

func (ssp shellScriptPricing) calculatePrice(ctx context.Context, gspec *dtypes.GroupSpec) (sdk.Coin, error) {
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
		return sdk.Coin{}, err
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
		return sdk.Coin{}, ctxErr
	}
	if err != nil {
		return sdk.Coin{}, err
	}

	// Decode the result
	decoder := json.NewDecoder(outputBuf)
	decoder.UseNumber()

	var priceNumber json.Number
	err = decoder.Decode(&priceNumber)
	if err != nil {
		return sdk.Coin{}, err
	}

	price, err := priceNumber.Int64()
	if err != nil {
		return sdk.Coin{}, ErrBidQuantityInvalid
	}

	if price == 0 {
		return sdk.Coin{}, ErrBidZero
	}

	if price < 0 {
		return sdk.Coin{}, ErrBidQuantityInvalid
	}

	return sdk.NewInt64Coin(denom, price), nil
}
