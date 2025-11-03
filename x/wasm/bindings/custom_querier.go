package bindings

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"

	wasmvmtypes "github.com/CosmWasm/wasmvm/v3/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	oracletypes "pkg.akt.dev/go/node/oracle/v1"
	oraclekeeper "pkg.akt.dev/node/v2/x/oracle/keeper"
)

// CustomQuerier returns a custom querier for Akash-specific queries from CosmWasm contracts.
// This enables contracts to query Akash chain state (like oracle module parameters)
// using the custom query mechanism defined in wasmd.
//
// The querier handles AkashQuery requests, which are JSON-encoded custom queries
// defined in contracts/*/src/querier.rs.
func CustomQuerier(oracleKeeper oraclekeeper.Keeper) func(ctx sdk.Context, request json.RawMessage) ([]byte, error) {
	return func(ctx sdk.Context, request json.RawMessage) ([]byte, error) {
		var query AkashQuery
		if err := json.Unmarshal(request, &query); err != nil {
			return nil, wasmvmtypes.InvalidRequest{Err: "failed to parse AkashQuery: " + err.Error()}
		}

		switch {
		case query.OracleParams != nil:
			return handleOracleParamsQuery(ctx, oracleKeeper)
		case query.GuardianSet != nil:
			return handleGuardianSetQuery(ctx, oracleKeeper)
		default:
			return nil, wasmvmtypes.UnsupportedRequest{Kind: "unknown akash query variant"}
		}
	}
}

// handleOracleParamsQuery handles the OracleParams query.
// It retrieves oracle module parameters and returns them in a format
// that matches the Rust OracleParamsResponse struct.
func handleOracleParamsQuery(ctx sdk.Context, keeper oraclekeeper.Keeper) ([]byte, error) {
	params, err := keeper.GetParams(ctx)
	if err != nil {
		// Don't leak internal error details to contracts for security
		return nil, wasmvmtypes.Unknown{}
	}

	// Convert proto params to JSON-serializable struct
	response := OracleParamsResponse{
		Params: convertOracleParams(params),
	}

	bz, err := json.Marshal(response)
	if err != nil {
		return nil, wasmvmtypes.Unknown{}
	}

	return bz, nil
}

// handleGuardianSetQuery handles the GuardianSet query.
// It retrieves the Wormhole guardian set from oracle params and returns it
// in a format that matches the Rust GuardianSetResponse struct.
func handleGuardianSetQuery(ctx sdk.Context, keeper oraclekeeper.Keeper) ([]byte, error) {
	params, err := keeper.GetParams(ctx)
	if err != nil {
		return nil, wasmvmtypes.Unknown{}
	}

	// Extract WormholeContractParams from FeedContractsParams Any slice
	var guardianAddresses []GuardianAddress
	for _, anyVal := range params.FeedContractsParams {
		if anyVal != nil && anyVal.TypeUrl == "/akash.oracle.v1.WormholeContractParams" {
			var wormholeParams oracletypes.WormholeContractParams
			if err := wormholeParams.Unmarshal(anyVal.Value); err == nil {
				// Convert hex-encoded guardian addresses to base64-encoded Binary
				for _, hexAddr := range wormholeParams.GuardianAddresses {
					// Decode hex string to bytes
					addrBytes, err := hex.DecodeString(hexAddr)
					if err != nil {
						continue
					}
					// Encode as base64 for CosmWasm Binary compatibility
					guardianAddresses = append(guardianAddresses, GuardianAddress{
						Bytes: base64.StdEncoding.EncodeToString(addrBytes),
					})
				}
				break
			}
		}
	}

	// Ensure addresses is never nil (Rust expects an array, not null)
	if guardianAddresses == nil {
		guardianAddresses = []GuardianAddress{}
	}

	response := GuardianSetResponse{
		Addresses:      guardianAddresses,
		ExpirationTime: 0, // Guardian set from governance never expires
	}

	bz, err := json.Marshal(response)
	if err != nil {
		return nil, wasmvmtypes.Unknown{}
	}

	return bz, nil
}

// convertOracleParams converts the proto Params type to the JSON-serializable OracleParams type.
// This ensures the JSON output matches what the Rust contract expects.
func convertOracleParams(params oracletypes.Params) OracleParams {
	result := OracleParams{
		Sources:                 params.Sources,
		MinPriceSources:         params.MinPriceSources,
		MaxPriceStalenessBlocks: params.MaxPriceStalenessBlocks,
		TwapWindow:              params.TwapWindow,
		MaxPriceDeviationBps:    params.MaxPriceDeviationBps,
	}

	// Ensure sources is never nil (Rust expects an array, not null)
	if result.Sources == nil {
		result.Sources = []string{}
	}

	// Extract PythContractParams and WormholeContractParams from FeedContractsParams Any slice.
	// The proto uses google.protobuf.Any for extensibility, so we need to
	// unpack it based on the type URL.
	for _, anyVal := range params.FeedContractsParams {
		if anyVal == nil {
			continue
		}
		switch anyVal.TypeUrl {
		case "/akash.oracle.v1.PythContractParams":
			var pythParams oracletypes.PythContractParams
			if err := pythParams.Unmarshal(anyVal.Value); err == nil {
				result.PythParams = &PythContractParams{
					AktPriceFeedId: pythParams.AktPriceFeedId,
				}
			}
		case "/akash.oracle.v1.WormholeContractParams":
			var wormholeParams oracletypes.WormholeContractParams
			if err := wormholeParams.Unmarshal(anyVal.Value); err == nil {
				result.WormholeParams = &WormholeContractParams{
					GuardianAddresses: wormholeParams.GuardianAddresses,
				}
			}
		}
	}

	return result
}
