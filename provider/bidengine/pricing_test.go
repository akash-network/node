package bidengine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"os/exec"
	"path"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/testutil"
	atypes "github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/unit"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

func Test_ScalePricingRejectsAllZero(t *testing.T) {
	pricing, err := MakeScalePricing(decimal.Zero, decimal.Zero, make(Storage), decimal.Zero)
	require.NotNil(t, err)
	require.Nil(t, pricing)
}

func Test_ScalePricingAcceptsOneForASingleScale(t *testing.T) {
	pricing, err := MakeScalePricing(decimal.NewFromInt(1), decimal.Zero, make(Storage), decimal.Zero)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	pricing, err = MakeScalePricing(decimal.Zero, decimal.NewFromInt(1), make(Storage), decimal.Zero)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	storageScale := Storage{
		"": decimal.NewFromInt(1),
	}
	pricing, err = MakeScalePricing(decimal.Zero, decimal.Zero, storageScale, decimal.Zero)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	pricing, err = MakeScalePricing(decimal.Zero, decimal.Zero, make(Storage), decimal.NewFromInt(1))
	require.NoError(t, err)
	require.NotNil(t, pricing)
}

func defaultGroupSpecCPUMem() *dtypes.GroupSpec {
	gspec := &dtypes.GroupSpec{
		Name:         "",
		Requirements: atypes.PlacementRequirements{},
		Resources:    make([]dtypes.Resource, 1),
	}

	cpu := atypes.CPU{}
	cpu.Units = atypes.NewResourceValue(11)

	memory := atypes.Memory{}
	memory.Quantity = atypes.NewResourceValue(10000)

	clusterResources := atypes.ResourceUnits{
		CPU:    &cpu,
		Memory: &memory,
	}

	price := sdk.NewInt64Coin("uakt", 23)
	resource := dtypes.Resource{
		Resources: clusterResources,
		Count:     1,
		Price:     price,
	}

	gspec.Resources[0] = resource
	gspec.Resources[0].Resources.Endpoints = make([]atypes.Endpoint, testutil.RandRangeInt(1, 10))
	return gspec
}

func defaultGroupSpec() *dtypes.GroupSpec {
	gspec := &dtypes.GroupSpec{
		Name:         "",
		Requirements: atypes.PlacementRequirements{},
		Resources:    make([]dtypes.Resource, 1),
	}

	cpu := atypes.CPU{}
	cpu.Units = atypes.NewResourceValue(11)

	memory := atypes.Memory{}
	memory.Quantity = atypes.NewResourceValue(10000)

	clusterResources := atypes.ResourceUnits{
		CPU:    &cpu,
		Memory: &memory,
		Storage: atypes.Volumes{
			atypes.Storage{
				Quantity: atypes.NewResourceValue(4096),
			},
		},
	}
	price := sdk.NewInt64Coin("uakt", 23)
	resource := dtypes.Resource{
		Resources: clusterResources,
		Count:     1,
		Price:     price,
	}

	gspec.Resources[0] = resource
	gspec.Resources[0].Resources.Endpoints = make([]atypes.Endpoint, testutil.RandRangeInt(1, 10))
	return gspec
}

func Test_ScalePricingFailsOnOverflow(t *testing.T) {
	storageScale := Storage{
		sdl.StorageEphemeral: decimal.NewFromInt(1),
	}

	pricing, err := MakeScalePricing(decimal.New(math.MaxInt64, 2), decimal.Zero, storageScale, decimal.Zero)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	price, err := pricing.CalculatePrice(context.Background(), testutil.AccAddress(t).String(), defaultGroupSpec())

	require.Equal(t, sdk.Coin{}, price)
	require.Equal(t, err, ErrBidQuantityInvalid)
}

func Test_ScalePricingOnCpu(t *testing.T) {
	cpuScale := decimal.NewFromInt(22)

	pricing, err := MakeScalePricing(cpuScale, decimal.Zero, make(Storage), decimal.Zero)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	gspec := defaultGroupSpecCPUMem()
	cpuQuantity := uint64(13)
	gspec.Resources[0].Resources.CPU.Units = atypes.NewResourceValue(cpuQuantity)
	price, err := pricing.CalculatePrice(context.Background(), testutil.AccAddress(t).String(), gspec)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	expectedPrice := testutil.AkashCoin(t, cpuScale.IntPart()*int64(cpuQuantity))
	require.Equal(t, expectedPrice, price)
}

func Test_ScalePricingOnCpuRoundsUpToOne(t *testing.T) {
	cpuScale, err := decimal.NewFromString("0.000001") // A small number
	require.NoError(t, err)
	pricing, err := MakeScalePricing(cpuScale, decimal.Zero, make(Storage), decimal.Zero)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	gspec := defaultGroupSpecCPUMem()
	cpuQuantity := testutil.RandRangeInt(10, 1000)
	gspec.Resources[0].Resources.CPU.Units = atypes.NewResourceValue(uint64(cpuQuantity))
	price, err := pricing.CalculatePrice(context.Background(), testutil.AccAddress(t).String(), gspec)
	require.NoError(t, err)

	// Implementation rounds up to 1
	expectedPrice := testutil.AkashCoin(t, 1)
	require.Equal(t, expectedPrice, price)

}

func Test_ScalePricingOnCpuRoundsUp(t *testing.T) {
	cpuScale, err := decimal.NewFromString("0.666667") // approximate 2/3
	require.NoError(t, err)
	pricing, err := MakeScalePricing(cpuScale, decimal.Zero, make(Storage), decimal.Zero)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	gspec := defaultGroupSpecCPUMem()
	cpuQuantity := testutil.RandRangeInt(10, 1000)
	gspec.Resources[0].Resources.CPU.Units = atypes.NewResourceValue(uint64(cpuQuantity))
	price, err := pricing.CalculatePrice(context.Background(), testutil.AccAddress(t).String(), gspec)
	require.NoError(t, err)

	// Implementation rounds up to nearest whole uakt
	expected := cpuScale.Mul(decimal.NewFromInt(int64(cpuQuantity))).Ceil()
	require.True(t, expected.IsPositive()) // sanity check expected value
	expectedPrice := testutil.AkashCoin(t, expected.IntPart())
	require.Equal(t, expectedPrice, price)
}

func Test_ScalePricingOnMemory(t *testing.T) {
	memoryScale := uint64(23)
	memoryPrice := decimal.NewFromInt(int64(memoryScale)).Mul(decimal.NewFromInt(unit.Mi))
	pricing, err := MakeScalePricing(decimal.Zero, memoryPrice, make(Storage), decimal.Zero)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	gspec := defaultGroupSpecCPUMem()
	memoryQuantity := uint64(123456)
	gspec.Resources[0].Resources.Memory.Quantity = atypes.NewResourceValue(memoryQuantity)
	price, err := pricing.CalculatePrice(context.Background(), testutil.AccAddress(t).String(), gspec)
	require.NoError(t, err)

	expectedPrice := testutil.AkashCoin(t, int64(memoryScale*memoryQuantity))
	require.Equal(t, expectedPrice, price)
}

func Test_ScalePricingOnMemoryRoundsUpA(t *testing.T) {
	memoryScale := uint64(123)
	memoryPrice := decimal.NewFromInt(int64(memoryScale))
	pricing, err := MakeScalePricing(decimal.Zero, memoryPrice, make(Storage), decimal.Zero)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	gspec := defaultGroupSpecCPUMem()
	// Make a resource exactly 1 byte greater than a megabyte
	memoryQuantity := uint64(unit.Mi + 1)
	gspec.Resources[0].Resources.Memory.Quantity = atypes.NewResourceValue(memoryQuantity)
	price, err := pricing.CalculatePrice(context.Background(), testutil.AccAddress(t).String(), gspec)
	require.NoError(t, err)

	// The pricing function cannot round down, so the price must exactly 1 uakt larger
	// than the scale provided
	expectedPrice := testutil.AkashCoin(t, int64(124))
	require.Equal(t, expectedPrice, price)
}

func Test_ScalePricingOnMemoryRoundsUpB(t *testing.T) {
	memoryScale := uint64(123)
	memoryPrice := decimal.NewFromInt(int64(memoryScale))
	pricing, err := MakeScalePricing(decimal.Zero, memoryPrice, make(Storage), decimal.Zero)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	gspec := defaultGroupSpecCPUMem()
	// Make a resource exactly 1 less byte less than two megabytes
	memoryQuantity := uint64(2*unit.Mi - 1)
	gspec.Resources[0].Resources.Memory.Quantity = atypes.NewResourceValue(memoryQuantity)
	price, err := pricing.CalculatePrice(context.Background(), testutil.AccAddress(t).String(), gspec)
	require.NoError(t, err)

	// The pricing function cannot round down, so the price must exactly twice the scale
	expectedPrice := testutil.AkashCoin(t, int64(246))
	require.Equal(t, expectedPrice, price)
}

func Test_ScalePricingOnMemoryRoundsUpFromZero(t *testing.T) {
	memoryScale := uint64(1) // 1 uakt per megabyte
	memoryPrice := decimal.NewFromInt(int64(memoryScale))
	pricing, err := MakeScalePricing(decimal.Zero, memoryPrice, make(Storage), decimal.Zero)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	gspec := defaultGroupSpecCPUMem()
	// Make a resource exactly 1 byte
	memoryQuantity := uint64(1)
	gspec.Resources[0].Resources.Memory.Quantity = atypes.NewResourceValue(memoryQuantity)
	price, err := pricing.CalculatePrice(context.Background(), testutil.AccAddress(t).String(), gspec)
	require.NoError(t, err)

	// The pricing function cannot round down, so the price must exactly 1 uakt
	expectedPrice := testutil.AkashCoin(t, int64(1))
	require.Equal(t, expectedPrice, price)
}

func Test_ScalePricingOnStorage(t *testing.T) {
	storageScale := uint64(24)
	storagePrice := Storage{
		sdl.StorageEphemeral: decimal.NewFromInt(int64(storageScale)).Mul(decimal.NewFromInt(unit.Mi)),
	}

	pricing, err := MakeScalePricing(decimal.Zero, decimal.Zero, storagePrice, decimal.Zero)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	gspec := defaultGroupSpec()
	storageQuantity := uint64(98765)
	gspec.Resources[0].Resources.Storage[0].Quantity = atypes.NewResourceValue(storageQuantity)
	price, err := pricing.CalculatePrice(context.Background(), testutil.AccAddress(t).String(), gspec)
	require.NoError(t, err)

	// one is added due to fractional rounding in the implementation
	expectedPrice := testutil.AkashCoin(t, int64(storageScale*storageQuantity)+1)
	require.Equal(t, expectedPrice, price)
}

func Test_ScalePricingByCountOfResources(t *testing.T) {
	storageScale := uint64(3)
	storagePrice := Storage{
		sdl.StorageEphemeral: decimal.NewFromInt(int64(storageScale)).Mul(decimal.NewFromInt(unit.Mi)),
	}

	pricing, err := MakeScalePricing(decimal.Zero, decimal.Zero, storagePrice, decimal.Zero)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	gspec := defaultGroupSpec()
	storageQuantity := uint64(111)
	gspec.Resources[0].Resources.Storage[0].Quantity = atypes.NewResourceValue(storageQuantity)
	firstPrice, err := pricing.CalculatePrice(context.Background(), testutil.AccAddress(t).String(), gspec)
	require.NoError(t, err)

	// one is added due to fractional rounding in the implementation
	firstExpectedPrice := testutil.AkashCoin(t, int64(storageScale*storageQuantity)+1)
	require.Equal(t, firstExpectedPrice, firstPrice)

	gspec.Resources[0].Count = 2
	secondPrice, err := pricing.CalculatePrice(context.Background(), testutil.AccAddress(t).String(), gspec)
	require.NoError(t, err)

	// one is added due to fractional rounding in the implementation
	secondExpectedPrice := testutil.AkashCoin(t, 2*int64(storageScale*storageQuantity)+1)
	require.Equal(t, secondExpectedPrice, secondPrice)
}

func Test_ScriptPricingRejectsEmptyStringForPath(t *testing.T) {
	pricing, err := MakeShellScriptPricing("", 1, 30000*time.Millisecond)
	require.NotNil(t, err)
	require.Nil(t, pricing)
	require.Contains(t, err.Error(), "empty string")
}

func Test_ScriptPricingRejectsProcessLimitOfZero(t *testing.T) {
	pricing, err := MakeShellScriptPricing("a", 0, 30000*time.Millisecond)
	require.NotNil(t, err)
	require.Nil(t, pricing)
	require.Contains(t, err.Error(), "process limit")
}

func Test_ScriptPricingRejectsTimeoutOfZero(t *testing.T) {
	pricing, err := MakeShellScriptPricing("a", 1, 0*time.Millisecond)
	require.NotNil(t, err)
	require.Nil(t, pricing)
	require.Contains(t, err.Error(), "runtime limit")
}

func Test_ScriptPricingFailsWhenScriptDoesNotExist(t *testing.T) {
	tempdir := t.TempDir()

	scriptPath := path.Join(tempdir, "test_script.sh")
	pricing, err := MakeShellScriptPricing(scriptPath, 1, 30000*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	_, err = pricing.CalculatePrice(context.Background(), testutil.AccAddress(t).String(), defaultGroupSpec())
	require.IsType(t, &os.PathError{}, err)
}

func Test_ScriptPricingFailsWhenScriptExitsNonZero(t *testing.T) {
	tempdir := t.TempDir()

	scriptPath := path.Join(tempdir, "test_script.sh")
	fout, err := os.OpenFile(scriptPath, os.O_WRONLY|os.O_CREATE, os.ModePerm)
	require.NoError(t, err)
	_, err = fout.WriteString("#!/bin/sh\nexit 1")
	require.NoError(t, err)
	err = fout.Close()
	require.NoError(t, err)

	pricing, err := MakeShellScriptPricing(scriptPath, 1, 30000*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	_, err = pricing.CalculatePrice(context.Background(), testutil.AccAddress(t).String(), defaultGroupSpec())
	require.IsType(t, &exec.ExitError{}, err)
}

func Test_ScriptPricingFailsWhenScriptExitsWithoutWritingResultToStdout(t *testing.T) {
	tempdir := t.TempDir()

	scriptPath := path.Join(tempdir, "test_script.sh")
	fout, err := os.OpenFile(scriptPath, os.O_WRONLY|os.O_CREATE, os.ModePerm)
	require.NoError(t, err)
	_, err = fout.WriteString("#!/bin/sh\nexit 0")
	require.NoError(t, err)
	err = fout.Close()
	require.NoError(t, err)

	pricing, err := MakeShellScriptPricing(scriptPath, 1, 30000*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	_, err = pricing.CalculatePrice(context.Background(), testutil.AccAddress(t).String(), defaultGroupSpec())
	require.Equal(t, io.EOF, err)
}

func Test_ScriptPricingFailsWhenScriptWritesZeroResult(t *testing.T) {
	tempdir := t.TempDir()

	scriptPath := path.Join(tempdir, "test_script.sh")
	fout, err := os.OpenFile(scriptPath, os.O_WRONLY|os.O_CREATE, os.ModePerm)
	require.NoError(t, err)
	_, err = fout.WriteString("#!/bin/sh\necho 0\nexit 0")
	require.NoError(t, err)
	err = fout.Close()
	require.NoError(t, err)

	pricing, err := MakeShellScriptPricing(scriptPath, 1, 30000*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	_, err = pricing.CalculatePrice(context.Background(), testutil.AccAddress(t).String(), defaultGroupSpec())
	require.Equal(t, ErrBidZero, err)
}

func Test_ScriptPricingFailsWhenScriptWritesNegativeResult(t *testing.T) {
	tempdir := t.TempDir()

	scriptPath := path.Join(tempdir, "test_script.sh")
	fout, err := os.OpenFile(scriptPath, os.O_WRONLY|os.O_CREATE, os.ModePerm)
	require.NoError(t, err)
	_, err = fout.WriteString("#!/bin/sh\necho -1\nexit 0")
	require.NoError(t, err)
	err = fout.Close()
	require.NoError(t, err)

	pricing, err := MakeShellScriptPricing(scriptPath, 1, 30000*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	_, err = pricing.CalculatePrice(context.Background(), testutil.AccAddress(t).String(), defaultGroupSpec())
	require.Equal(t, ErrBidQuantityInvalid, err)
}

func Test_ScriptPricingFailsWhenScriptWritesFractionalResult(t *testing.T) {
	tempdir := t.TempDir()

	scriptPath := path.Join(tempdir, "test_script.sh")
	fout, err := os.OpenFile(scriptPath, os.O_WRONLY|os.O_CREATE, os.ModePerm)
	require.NoError(t, err)
	_, err = fout.WriteString("#!/bin/sh\necho 1.5\nexit 0")
	require.NoError(t, err)
	err = fout.Close()
	require.NoError(t, err)

	pricing, err := MakeShellScriptPricing(scriptPath, 1, 30000*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	_, err = pricing.CalculatePrice(context.Background(), testutil.AccAddress(t).String(), defaultGroupSpec())
	require.Equal(t, ErrBidQuantityInvalid, err)
}

func Test_ScriptPricingFailsWhenScriptWritesOverflowResult(t *testing.T) {
	tempdir := t.TempDir()

	scriptPath := path.Join(tempdir, "test_script.sh")
	fout, err := os.OpenFile(scriptPath, os.O_WRONLY|os.O_CREATE, os.ModePerm)
	require.NoError(t, err)
	// Value below is 2^63, which does not fit in int64
	_, err = fout.WriteString("#!/bin/sh\necho 9223372036854775808\nexit 0")
	require.NoError(t, err)
	err = fout.Close()
	require.NoError(t, err)

	pricing, err := MakeShellScriptPricing(scriptPath, 1, 30000*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	_, err = pricing.CalculatePrice(context.Background(), testutil.AccAddress(t).String(), defaultGroupSpec())
	require.Equal(t, ErrBidQuantityInvalid, err)
}

func Test_ScriptPricingReturnsResultFromScript(t *testing.T) {
	tempdir := t.TempDir()

	scriptPath := path.Join(tempdir, "test_script.sh")
	fout, err := os.OpenFile(scriptPath, os.O_WRONLY|os.O_CREATE, os.ModePerm)
	require.NoError(t, err)
	// Value must not have a decimal
	_, err = fout.WriteString("#!/bin/sh\necho 132\nexit 0")
	require.NoError(t, err)
	err = fout.Close()
	require.NoError(t, err)

	pricing, err := MakeShellScriptPricing(scriptPath, 1, 30000*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	price, err := pricing.CalculatePrice(context.Background(), testutil.AccAddress(t).String(), defaultGroupSpec())
	require.NoError(t, err)
	require.Equal(t, "uakt", price.Denom)
	require.Equal(t, int64(132), price.Amount.Int64())
}

func Test_ScriptPricingDoesNotExhaustSemaphore(t *testing.T) {
	tempdir := t.TempDir()

	scriptPath := path.Join(tempdir, "test_script.sh")
	fout, err := os.OpenFile(scriptPath, os.O_WRONLY|os.O_CREATE, os.ModePerm)
	require.NoError(t, err)
	_, err = fout.WriteString("#!/bin/sh\necho 1\nexit 0")
	require.NoError(t, err)
	err = fout.Close()
	require.NoError(t, err)

	pricing, err := MakeShellScriptPricing(scriptPath, 10, 30000*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	// run the script lots of time to make sure the channel used
	// as a semaphore always has things returned to it
	for i := 0; i != 111; i++ {
		_, err = pricing.CalculatePrice(context.Background(), testutil.AccAddress(t).String(), defaultGroupSpec())
		require.NoError(t, err)
	}
}

func Test_ScriptPricingStopsByContext(t *testing.T) {
	tempdir := t.TempDir()

	scriptPath := path.Join(tempdir, "test_script.sh")
	fout, err := os.OpenFile(scriptPath, os.O_WRONLY|os.O_CREATE, os.ModePerm)
	require.NoError(t, err)
	_, err = fout.WriteString("#!/bin/sh\nsleep 4\n")
	require.NoError(t, err)
	err = fout.Close()
	require.NoError(t, err)

	pricing, err := MakeShellScriptPricing(scriptPath, 10, 5000*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = pricing.CalculatePrice(ctx, testutil.AccAddress(t).String(), defaultGroupSpec())
	require.Error(t, err)
	require.Equal(t, context.Canceled, err)
}

func Test_ScriptPricingStopsByTimeout(t *testing.T) {
	_, err := os.Stat("/bin/bash")
	if os.IsNotExist(err) {
		t.Skip("cannot run without bash shell")
	}
	require.NoError(t, err)
	tempdir := t.TempDir()

	scriptPath := path.Join(tempdir, "test_script.sh")
	fout, err := os.OpenFile(scriptPath, os.O_WRONLY|os.O_CREATE, os.ModePerm)
	require.NoError(t, err)
	_, err = fout.WriteString("#!/bin/bash\nsleep 10\n")
	require.NoError(t, err)
	err = fout.Close()
	require.NoError(t, err)

	pricing, err := MakeShellScriptPricing(scriptPath, 10, 1*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	ctx := context.Background()
	_, err = pricing.CalculatePrice(ctx, testutil.AccAddress(t).String(), defaultGroupSpec())
	require.Error(t, err)
	require.Equal(t, context.DeadlineExceeded, err)
}

func Test_ScriptPricingWritesJsonToStdin(t *testing.T) {
	tempdir := t.TempDir()

	scriptPath := path.Join(tempdir, "test_script.sh")
	jsonPath := path.Join(tempdir, "stdin.json")
	fout, err := os.OpenFile(scriptPath, os.O_WRONLY|os.O_CREATE, os.ModePerm)
	require.NoError(t, err)
	// Use cat to dump stdin into a file
	_, err = fout.WriteString(fmt.Sprintf("#!/bin/sh\ncat > %q\necho 1\nexit 0", jsonPath))
	require.NoError(t, err)
	err = fout.Close()
	require.NoError(t, err)

	pricing, err := MakeShellScriptPricing(scriptPath, 1, 30000*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	gspec := defaultGroupSpec()
	price, err := pricing.CalculatePrice(context.Background(), testutil.AccAddress(t).String(), gspec)
	require.NoError(t, err)
	require.Equal(t, "uakt", price.Denom)
	require.Equal(t, int64(1), price.Amount.Int64())
	// Open the file and make sure it has the JSON
	fin, err := os.Open(jsonPath)
	require.NoError(t, err)
	defer func() {
		_ = fin.Close()
	}()
	decoder := json.NewDecoder(fin)
	data := make([]dataForScriptElement, 0)
	err = decoder.Decode(&data)
	require.NoError(t, err)

	require.Len(t, data, len(gspec.Resources))

	for i, r := range gspec.Resources {
		require.Equal(t, r.Resources.CPU.Units.Val.Uint64(), data[i].CPU)
		require.Equal(t, r.Resources.Memory.Quantity.Val.Uint64(), data[i].Memory)
		require.Equal(t, r.Resources.Storage[0].Quantity.Val.Uint64(), data[i].Storage["default"])
		require.Equal(t, r.Count, data[i].Count)
		require.Equal(t, len(r.Resources.Endpoints), data[i].EndpointQuantity)
	}
}

func TestRationalToIntConversion(t *testing.T) {
	x := ceilBigRatToBigInt(big.NewRat(0, 1))
	require.Equal(t, big.NewInt(0), x)

	y := ceilBigRatToBigInt(big.NewRat(1, 1))
	require.Equal(t, big.NewInt(1), y)

	z := ceilBigRatToBigInt(big.NewRat(1, 2))
	require.Equal(t, big.NewInt(1), z)

	a := ceilBigRatToBigInt(big.NewRat(3, 2))
	require.Equal(t, big.NewInt(2), a)
}
