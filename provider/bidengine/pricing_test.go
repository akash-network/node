package bidengine

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/testutil"
	atypes "github.com/ovrclk/akash/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/stretchr/testify/require"
	io "io"
	"math"
	"os"
	"os/exec"
	"path"
	"testing"
	"time"
)

func Test_ScalePricingRejectsAllZero(t *testing.T) {
	pricing, err := MakeScalePricing(0, 0, 0, 0)
	require.NotNil(t, err)
	require.Nil(t, pricing)
}

func Test_ScalePricingAcceptsOneForASingleScale(t *testing.T) {
	pricing, err := MakeScalePricing(1, 0, 0, 0)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	pricing, err = MakeScalePricing(0, 1, 0, 0)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	pricing, err = MakeScalePricing(0, 0, 1, 0)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	pricing, err = MakeScalePricing(0, 0, 0, 1)
	require.NoError(t, err)
	require.NotNil(t, pricing)
}

func defaultGroupSpec() *dtypes.GroupSpec {
	gspec := &dtypes.GroupSpec{
		Name:             "",
		Requirements:     nil,
		Resources:        make([]dtypes.Resource, 1),
		OrderBidDuration: 0,
	}

	cpu := atypes.CPU{}
	cpu.Units = atypes.NewResourceValue(11)

	memory := atypes.Memory{}
	memory.Quantity = atypes.NewResourceValue(10000)

	storage := atypes.Storage{}
	storage.Quantity = atypes.NewResourceValue(4096)

	clusterResources := atypes.ResourceUnits{
		CPU:     &cpu,
		Memory:  &memory,
		Storage: &storage,
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
	pricing, err := MakeScalePricing(math.MaxUint64, 0, 0, 0)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	price, err := pricing.calculatePrice(context.Background(), defaultGroupSpec())

	require.Equal(t, sdk.Coin{}, price)
	require.Equal(t, err, ErrBidQuantityInvalid)
}

func Test_ScalePricingOnCpu(t *testing.T) {
	cpuScale := uint64(22)
	pricing, err := MakeScalePricing(cpuScale, 0, 0, 0)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	gspec := defaultGroupSpec()
	cpuQuantity := uint64(13)
	gspec.Resources[0].Resources.CPU.Units = atypes.NewResourceValue(cpuQuantity)
	price, err := pricing.calculatePrice(context.Background(), gspec)

	expectedPrice := testutil.AkashCoin(t, int64(cpuScale*cpuQuantity))
	require.Equal(t, expectedPrice, price)
	require.NoError(t, err)
}

func Test_ScalePricingOnMemory(t *testing.T) {
	memoryScale := uint64(23)
	pricing, err := MakeScalePricing(0, memoryScale, 0, 0)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	gspec := defaultGroupSpec()
	memoryQuantity := uint64(123456)
	gspec.Resources[0].Resources.Memory.Quantity = atypes.NewResourceValue(memoryQuantity)
	price, err := pricing.calculatePrice(context.Background(), gspec)

	expectedPrice := testutil.AkashCoin(t, int64(memoryScale*memoryQuantity))
	require.Equal(t, expectedPrice, price)
	require.NoError(t, err)
}

func Test_ScalePricingOnStorage(t *testing.T) {
	storageScale := uint64(24)
	pricing, err := MakeScalePricing(0, 0, storageScale, 0)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	gspec := defaultGroupSpec()
	storageQuantity := uint64(98765)
	gspec.Resources[0].Resources.Storage.Quantity = atypes.NewResourceValue(storageQuantity)
	price, err := pricing.calculatePrice(context.Background(), gspec)

	expectedPrice := testutil.AkashCoin(t, int64(storageScale*storageQuantity))
	require.Equal(t, expectedPrice, price)
	require.NoError(t, err)
}

func Test_ScalePricingByCountOfResources(t *testing.T) {
	storageScale := uint64(3)
	pricing, err := MakeScalePricing(0, 0, storageScale, 0)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	gspec := defaultGroupSpec()
	storageQuantity := uint64(111)
	gspec.Resources[0].Resources.Storage.Quantity = atypes.NewResourceValue(storageQuantity)
	firstPrice, err := pricing.calculatePrice(context.Background(), gspec)

	firstExpectedPrice := testutil.AkashCoin(t, int64(storageScale*storageQuantity))
	require.Equal(t, firstExpectedPrice, firstPrice)
	require.NoError(t, err)

	gspec.Resources[0].Count = 2
	secondPrice, err := pricing.calculatePrice(context.Background(), gspec)
	secondExpectedPrice := testutil.AkashCoin(t, 2*int64(storageScale*storageQuantity))
	require.Equal(t, secondExpectedPrice, secondPrice)
	require.NoError(t, err)
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

	_, err = pricing.calculatePrice(context.Background(), defaultGroupSpec())
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

	_, err = pricing.calculatePrice(context.Background(), defaultGroupSpec())
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

	_, err = pricing.calculatePrice(context.Background(), defaultGroupSpec())
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

	_, err = pricing.calculatePrice(context.Background(), defaultGroupSpec())
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

	_, err = pricing.calculatePrice(context.Background(), defaultGroupSpec())
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

	_, err = pricing.calculatePrice(context.Background(), defaultGroupSpec())
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

	_, err = pricing.calculatePrice(context.Background(), defaultGroupSpec())
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

	price, err := pricing.calculatePrice(context.Background(), defaultGroupSpec())
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
		_, err = pricing.calculatePrice(context.Background(), defaultGroupSpec())
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
	_, err = pricing.calculatePrice(ctx, defaultGroupSpec())
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
	_, err = pricing.calculatePrice(ctx, defaultGroupSpec())
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
	price, err := pricing.calculatePrice(context.Background(), gspec)
	require.NoError(t, err)
	require.Equal(t, "uakt", price.Denom)
	require.Equal(t, int64(1), price.Amount.Int64())
	// Open the file and make sure it has the JSON
	fin, err := os.Open(jsonPath)
	require.NoError(t, err)
	defer fin.Close()
	decoder := json.NewDecoder(fin)
	data := make([]dataForScriptElement, 0)
	err = decoder.Decode(&data)
	require.NoError(t, err)

	require.Len(t, data, len(gspec.Resources))

	for i, r := range gspec.Resources {
		require.Equal(t, r.Resources.CPU.Units.Val.Uint64(), data[i].CPU)
		require.Equal(t, r.Resources.Memory.Quantity.Val.Uint64(), data[i].Memory)
		require.Equal(t, r.Resources.Storage.Quantity.Val.Uint64(), data[i].Storage)
		require.Equal(t, r.Count, data[i].Count)
		require.Equal(t, len(r.Resources.Endpoints), data[i].EndpointQuantity)
	}
}
