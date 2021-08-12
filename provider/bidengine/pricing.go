package bidengine

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"os/exec"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"

	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/types/unit"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

type BidPricingStrategy interface {
	CalculatePrice(ctx context.Context, owner string, gspec *dtypes.GroupSpec) (sdk.Coin, error)
}

const denom = "uakt"

var (
	errAllScalesZero               = errors.New("at least one bid price must be a non-zero number")
	errNoPriceScaleForStorageClass = errors.New("no pricing configured for storage class")
)

type Storage map[string]decimal.Decimal

func (ss Storage) IsAnyZero() bool {
	if len(ss) == 0 {
		return true
	}

	for _, val := range ss {
		if val.IsZero() {
			return true
		}
	}

	return false
}

func (ss Storage) IsAnyNegative() bool {
	for _, val := range ss {
		if val.IsNegative() {
			return true
		}
	}

	return false
}

// AllLessThenOrEqual check all storage classes fit into max limits
// note better have dedicated limits for each class
func (ss Storage) AllLessThenOrEqual(val decimal.Decimal) bool {
	for _, storage := range ss {
		if !storage.LessThanOrEqual(val) {
			return false
		}
	}

	return true
}

type scalePricing struct {
	cpuScale      decimal.Decimal
	memoryScale   decimal.Decimal
	storageScale  Storage
	endpointScale decimal.Decimal
}

func MakeScalePricing(
	cpuScale decimal.Decimal,
	memoryScale decimal.Decimal,
	storageScale Storage,
	endpointScale decimal.Decimal) (BidPricingStrategy, error) {

	if cpuScale.IsZero() && memoryScale.IsZero() && storageScale.IsAnyZero() && endpointScale.IsZero() {
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

func (fp scalePricing) CalculatePrice(_ context.Context, _ string, gspec *dtypes.GroupSpec) (sdk.Coin, error) {
	// Use unlimited precision math here.
	// Otherwise a correctly crafted order could create a cost of '1' given
	// a possible configuration
	cpuTotal := decimal.NewFromInt(0)
	memoryTotal := decimal.NewFromInt(0)
	storageTotal := make(Storage)

	for k := range fp.storageScale {
		storageTotal[k] = decimal.NewFromInt(0)
	}

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

		for _, storage := range group.Resources.Storage {
			storageQuantity := decimal.NewFromBigInt(storage.Quantity.Val.BigInt(), 0)
			storageQuantity = storageQuantity.Mul(groupCount)

			storageClass := sdl.StorageEphemeral
			attr := storage.Attributes.Find(sdl.StorageAttributePersistent)
			if isPersistent, _ := attr.AsBool(); isPersistent {
				attr = storage.Attributes.Find(sdl.StorageAttributeClass)
				if class, set := attr.AsString(); set {
					storageClass = class
				}
			}

			total, exists := storageTotal[storageClass]

			if !exists {
				return sdk.Coin{}, errors.Wrapf(errNoPriceScaleForStorageClass, storageClass)
			}

			total = total.Add(storageQuantity)

			storageTotal[storageClass] = total
		}

		endpointQuantity := decimal.NewFromInt(int64(len(group.Resources.Endpoints)))
		endpointTotal = endpointTotal.Add(endpointQuantity)
	}

	cpuTotal = cpuTotal.Mul(fp.cpuScale)

	mebibytes := decimal.NewFromInt(unit.Mi)

	memoryTotal = memoryTotal.Div(mebibytes)
	memoryTotal = memoryTotal.Mul(fp.memoryScale)

	for class, total := range storageTotal {
		total = total.Div(mebibytes)

		// at this point presence of class in storageScale has been validated
		total = total.Mul(fp.storageScale[class])

		storageTotal[class] = total
	}

	endpointTotal = endpointTotal.Mul(fp.endpointScale)

	maxAllowedValue := decimal.NewFromInt(math.MaxInt64)
	// Each quantity must be non negative
	// and fit into an Int64
	if cpuTotal.IsNegative() || !cpuTotal.LessThanOrEqual(maxAllowedValue) ||
		memoryTotal.IsNegative() || !memoryTotal.LessThanOrEqual(maxAllowedValue) ||
		storageTotal.IsAnyNegative() || !storageTotal.AllLessThenOrEqual(maxAllowedValue) ||
		endpointTotal.IsNegative() || !endpointTotal.LessThanOrEqual(maxAllowedValue) {
		return sdk.Coin{}, ErrBidQuantityInvalid
	}

	totalCost := cpuTotal
	totalCost = totalCost.Add(memoryTotal)
	for _, total := range storageTotal {
		totalCost = totalCost.Add(total)
	}
	totalCost = totalCost.Add(endpointTotal)
	totalCost = totalCost.Ceil() // Round upwards to get an integer

	if totalCost.IsNegative() || !totalCost.LessThanOrEqual(maxAllowedValue) {
		return sdk.Coin{}, ErrBidQuantityInvalid
	}

	if totalCost.IsZero() {
		// Return an error indicating we can't bid with a cost of zero
		return sdk.Coin{}, ErrBidZero
	}

	cost := sdk.NewCoin(denom, sdk.NewIntFromBigInt(totalCost.BigInt()))

	return cost, nil
}

type randomRangePricing int

func MakeRandomRangePricing() (BidPricingStrategy, error) {
	return randomRangePricing(0), nil
}

func (randomRangePricing) CalculatePrice(_ context.Context, _ string, gspec *dtypes.GroupSpec) (sdk.Coin, error) {
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
	// it isn't a good idea to open thousands of processes.
	for i := uint(0); i != processLimit; i++ {
		result.processLimit <- 0
	}

	return result, nil
}

type dataForScriptElement struct {
	Memory           uint64            `json:"memory"`
	CPU              uint64            `json:"cpu"`
	Storage          map[string]uint64 `json:"storage"`
	Count            uint32            `json:"count"`
	EndpointQuantity int               `json:"endpoint_quantity"`
}

func (ssp shellScriptPricing) CalculatePrice(ctx context.Context, owner string, gspec *dtypes.GroupSpec) (sdk.Coin, error) {
	buf := &bytes.Buffer{}

	dataForScript := make([]dataForScriptElement, len(gspec.Resources))

	// iterate over everything & sum it up
	for i, group := range gspec.Resources {
		groupCount := group.Count
		cpuQuantity := group.Resources.CPU.Units.Val.Uint64()
		memoryQuantity := group.Resources.Memory.Quantity.Value()
		storageQuantity := make(map[string]uint64)

		for _, storage := range group.Resources.Storage {
			class := "default"
			storageQuantity[class] = storage.Quantity.Val.Uint64()
		}

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

	subprocEnv := os.Environ()
	subprocEnv = append(subprocEnv, fmt.Sprintf("AKASH_OWNER=%s", owner))
	cmd.Env = subprocEnv

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
