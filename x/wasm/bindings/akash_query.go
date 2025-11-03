package bindings

// AkashQuery represents custom Akash chain queries from CosmWasm contracts.
// This enum must match the Rust definition in contracts/*/src/querier.rs
//
// The JSON serialization uses snake_case to match Rust's serde default.
type AkashQuery struct {
	// OracleParams queries the oracle module parameters
	OracleParams *OracleParamsQuery `json:"oracle_params,omitempty"`
	// GuardianSet queries the Wormhole guardian set from oracle params
	GuardianSet *GuardianSetQuery `json:"guardian_set,omitempty"`
}

// OracleParamsQuery is the query payload for oracle params.
// It's an empty struct as the Rust side uses `OracleParams {}`.
type OracleParamsQuery struct{}

// GuardianSetQuery is the query payload for guardian set.
// It's an empty struct as the Rust side uses `GuardianSet {}`.
type GuardianSetQuery struct{}

// OracleParamsResponse is the response wrapper for oracle params query.
// Must match: contracts/pyth/src/querier.rs::OracleParamsResponse
type OracleParamsResponse struct {
	Params OracleParams `json:"params"`
}

// GuardianSetResponse is the response wrapper for guardian set query.
// Must match: contracts/wormhole/src/querier.rs::GuardianSetResponse
type GuardianSetResponse struct {
	// Addresses is the list of guardian addresses (20 bytes each, hex encoded)
	Addresses []GuardianAddress `json:"addresses"`
	// ExpirationTime is when this guardian set expires (0 = never)
	ExpirationTime uint64 `json:"expiration_time"`
}

// GuardianAddress represents a Wormhole guardian's Ethereum-style address.
// The address is 20 bytes, stored as base64-encoded Binary in Rust.
type GuardianAddress struct {
	// Bytes is the 20-byte guardian address (base64 encoded for JSON)
	Bytes string `json:"bytes"`
}

// OracleParams represents the oracle module parameters.
// Must match: contracts/pyth/src/querier.rs::OracleParams
// and proto: akash.oracle.v1.Params
type OracleParams struct {
	// Sources contains addresses allowed to write prices (contract addresses)
	Sources []string `json:"sources"`
	// MinPriceSources is the minimum number of price sources required
	MinPriceSources uint32 `json:"min_price_sources"`
	// MaxPriceStalenessBlocks is the maximum price staleness in blocks
	MaxPriceStalenessBlocks int64 `json:"max_price_staleness_blocks"`
	// TwapWindow is the TWAP window in blocks
	TwapWindow int64 `json:"twap_window"`
	// MaxPriceDeviationBps is the maximum price deviation in basis points
	MaxPriceDeviationBps uint64 `json:"max_price_deviation_bps"`
	// PythParams contains Pyth-specific configuration (optional)
	PythParams *PythContractParams `json:"pyth_params,omitempty"`
	// WormholeParams contains Wormhole-specific configuration (optional)
	WormholeParams *WormholeContractParams `json:"wormhole_params,omitempty"`
}

// PythContractParams contains configuration for Pyth price feeds.
// Must match: contracts/pyth/src/querier.rs::PythContractParams
// and proto: akash.oracle.v1.PythContractParams
type PythContractParams struct {
	// AktPriceFeedId is the Pyth price feed identifier for AKT/USD
	AktPriceFeedId string `json:"akt_price_feed_id"`
}

// WormholeContractParams contains configuration for Wormhole guardian set.
// Must match: contracts/wormhole/src/querier.rs::WormholeContractParams
// and proto: akash.oracle.v1.WormholeContractParams
type WormholeContractParams struct {
	// GuardianAddresses is the list of guardian addresses (20 bytes each, hex encoded)
	GuardianAddresses []string `json:"guardian_addresses"`
}
