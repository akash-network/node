package v2_2_0_test

import (
	"math"
	"testing"

	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmvm "github.com/CosmWasm/wasmvm/v3"
	wasmvmtypes "github.com/CosmWasm/wasmvm/v3/types"
	"github.com/stretchr/testify/require"

	akash "pkg.akt.dev/node/v2/app"
	v220 "pkg.akt.dev/node/v2/upgrades/software/v2.2.0"
)

// storePythVaa runs libwasmvm static validation + compilation for the contract stored by
// the v2.2.0 upgrade handler, using the given limits.
func storePythVaa(t *testing.T, limits wasmvmtypes.WasmLimits) error {
	t.Helper()

	vm, err := wasmvm.NewVMWithConfig(wasmvmtypes.VMConfig{
		WasmLimits: limits,
		Cache: wasmvmtypes.CacheOptions{
			BaseDir:                  t.TempDir(),
			AvailableCapabilities:    append(wasmkeeper.BuiltInCapabilities(), "akash"),
			MemoryCacheSizeBytes:     wasmvmtypes.NewSize(0),        // in-memory cache disabled
			InstanceMemoryLimitBytes: wasmvmtypes.NewSize(32 << 20), // 32MiB
		},
	}, false)
	require.NoError(t, err)
	t.Cleanup(vm.Cleanup)

	_, _, err = vm.StoreCode(v220.PythVaaContract, math.MaxUint64)
	return err
}

func TestPythVaaContractPassesCanonicalWasmLimits(t *testing.T) {
	require.NoError(t, storePythVaa(t, akash.DefaultWasmLimits().ToVM()))
}

// Documents the original failure: upstream libwasmvm defaults allow only 100 locals.
func TestPythVaaContractFailsUpstreamWasmLimits(t *testing.T) {
	err := storePythVaa(t, wasmvmtypes.WasmLimits{})
	require.ErrorContains(t, err, "more than 100 locals: 137")
}
