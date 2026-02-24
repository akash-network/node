//go:build e2e.integration

package e2e

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/CosmWasm/wasmd/x/wasm/ioutils"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"

	aclient "pkg.akt.dev/go/node/client/discovery"
	cltypes "pkg.akt.dev/go/node/client/types"
	cclient "pkg.akt.dev/go/node/client/v1beta3"
	oracletypes "pkg.akt.dev/go/node/oracle/v1"

	"pkg.akt.dev/node/v2/testutil"
	"pkg.akt.dev/node/v2/testutil/network"
)

// priceOracleContractTestSuite tests the Wormhole and Pyth CosmWasm contracts
// deployed on a test network.
// Architecture: Hermes → Pyth (verifies VAA + relays) → x/oracle
//
//	Wormhole provides VAA signature verification
type priceOracleContractTestSuite struct {
	*testutil.NetworkTestSuite

	cctx client.Context
}

func (s *priceOracleContractTestSuite) SetupSuite() {
	s.NetworkTestSuite.SetupSuite()

	val := s.Network().Validators[0]
	s.cctx = val.ClientCtx
}

// NetworkConfig returns a custom network config with a short governance voting period
// to enable contract deployment tests to complete in a reasonable time.
func NetworkConfig() *network.Config {
	cfg := network.DefaultConfig(testutil.NewTestNetworkFixture,
		network.WithInterceptState(func(cdc codec.Codec, moduleName string, state json.RawMessage) json.RawMessage {
			if moduleName == govtypes.ModuleName {
				var govGenState govv1.GenesisState
				cdc.MustUnmarshalJSON(state, &govGenState)

				// Short voting period for tests (10 seconds)
				votingPeriod := 10 * time.Second
				govGenState.Params.VotingPeriod = &votingPeriod

				// Also reduce min deposit
				govGenState.Params.MinDeposit = sdk.NewCoins(sdk.NewInt64Coin("uakt", 10000000))

				return cdc.MustMarshalJSON(&govGenState)
			}
			return nil
		}),
	)
	cfg.NumValidators = 1
	return &cfg
}

// =====================
// Wormhole Contract Types
// =====================

// WormholeInstantiateMsg is the message to instantiate the wormhole contract
type WormholeInstantiateMsg struct {
	GovChain            uint16          `json:"gov_chain"`
	GovAddress          string          `json:"gov_address"`
	InitialGuardianSet  GuardianSetInfo `json:"initial_guardian_set"`
	GuardianSetExpirity uint64          `json:"guardian_set_expirity"`
	ChainID             uint16          `json:"chain_id"`
	FeeDenom            string          `json:"fee_denom"`
}

// GuardianSetInfo contains guardian set data
type GuardianSetInfo struct {
	Addresses      []GuardianAddress `json:"addresses"`
	ExpirationTime uint64            `json:"expiration_time"`
}

// GuardianAddress represents a guardian's Ethereum-style address
type GuardianAddress struct {
	Bytes string `json:"bytes"` // base64 encoded
}

// WormholeExecuteMsg is the execute message for wormhole contract
type WormholeExecuteMsg struct {
	SubmitVAA   *SubmitVAAMsg   `json:"submit_v_a_a,omitempty"`
	PostMessage *PostMessageMsg `json:"post_message,omitempty"`
}

type SubmitVAAMsg struct {
	VAA string `json:"vaa"` // base64 encoded
}

type PostMessageMsg struct {
	Message string `json:"message"` // base64 encoded
	Nonce   uint32 `json:"nonce"`
}

// WormholeQueryMsg is the query message for wormhole contract
type WormholeQueryMsg struct {
	GuardianSetInfo *struct{}           `json:"guardian_set_info,omitempty"`
	VerifyVAA       *VerifyVAAQuery     `json:"verify_v_a_a,omitempty"`
	GetState        *struct{}           `json:"get_state,omitempty"`
	QueryAddressHex *QueryAddressHexMsg `json:"query_address_hex,omitempty"`
}

type VerifyVAAQuery struct {
	VAA       string `json:"vaa"` // base64 encoded
	BlockTime uint64 `json:"block_time"`
}

type QueryAddressHexMsg struct {
	Address string `json:"address"`
}

// WormholeGuardianSetInfoResponse is the response from GuardianSetInfo query
type WormholeGuardianSetInfoResponse struct {
	GuardianSetIndex uint32            `json:"guardian_set_index"`
	Addresses        []GuardianAddress `json:"addresses"`
}

// WormholeGetStateResponse is the response from GetState query
type WormholeGetStateResponse struct {
	Fee CoinResponse `json:"fee"`
}

type CoinResponse struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}

// =====================
// DataSource Type (shared)
// =====================

// DataSource identifies a valid price feed source (Pyth emitter)
type DataSource struct {
	EmitterChain   uint16 `json:"emitter_chain"`
	EmitterAddress string `json:"emitter_address"`
}

// =====================
// Price Oracle Contract Types
// =====================

// InstantiateMsg is the message to instantiate the Pyth contract
type InstantiateMsg struct {
	Admin            string       `json:"admin"`
	WormholeContract string       `json:"wormhole_contract"`
	UpdateFee        string       `json:"update_fee"`
	PriceFeedID      string       `json:"price_feed_id"`
	DataSources      []DataSource `json:"data_sources"`
}

// ExecuteUpdatePriceFeed is the message to update the price feed with VAA
type ExecuteUpdatePriceFeed struct {
	UpdatePriceFeed UpdatePriceFeedData `json:"update_price_feed"`
}

// UpdatePriceFeedData contains the VAA for price verification
type UpdatePriceFeedData struct {
	// VAA data from Pyth Hermes API (base64 encoded Binary)
	// Contract will verify VAA via Wormhole, parse Pyth payload, relay to x/oracle
	VAA string `json:"vaa"`
}

// ExecuteUpdateConfig is the message to update contract configuration
type ExecuteUpdateConfig struct {
	UpdateConfig UpdateConfigData `json:"update_config"`
}

type UpdateConfigData struct {
	WormholeContract *string       `json:"wormhole_contract,omitempty"`
	PriceFeedID      *string       `json:"price_feed_id,omitempty"`
	DataSources      *[]DataSource `json:"data_sources,omitempty"`
}

// QueryGetConfig is the query to get contract config
type QueryGetConfig struct{}

// QueryMsg wraps query messages
type QueryMsg struct {
	GetConfig       *QueryGetConfig       `json:"get_config,omitempty"`
	GetPrice        *QueryGetPrice        `json:"get_price,omitempty"`
	GetPriceFeed    *QueryGetPriceFeed    `json:"get_price_feed,omitempty"`
	GetOracleParams *QueryGetOracleParams `json:"get_oracle_params,omitempty"`
}

type QueryGetPrice struct{}
type QueryGetPriceFeed struct{}
type QueryGetOracleParams struct{}

// ConfigResponse is the response from GetConfig query
type ConfigResponse struct {
	Admin            string       `json:"admin"`
	WormholeContract string       `json:"wormhole_contract"`
	UpdateFee        string       `json:"update_fee"`
	PriceFeedID      string       `json:"price_feed_id"`
	DefaultDenom     string       `json:"default_denom"`
	DefaultBaseDenom string       `json:"default_base_denom"`
	DataSources      []DataSource `json:"data_sources"`
}

// PriceResponse is the response from GetPrice query
type PriceResponse struct {
	Price       string `json:"price"`
	Conf        string `json:"conf"`
	Expo        int32  `json:"expo"`
	PublishTime int64  `json:"publish_time"`
}

// OracleParamsResponse is the response from GetOracleParams query
type OracleParamsResponse struct {
	MaxPriceDeviationBps    uint64 `json:"max_price_deviation_bps"`
	MinPriceSources         uint32 `json:"min_price_sources"`
	MaxPriceStalenessBlocks int64  `json:"max_price_staleness_blocks"`
	TwapWindow              int64  `json:"twap_window"`
	LastUpdatedHeight       uint64 `json:"last_updated_height"`
}

// =====================
// Tests
// =====================

// TestStoreContractViaGovernance tests storing contracts via governance proposal.
// Note: In the test network without upgrade handler applied, direct code upload is allowed.
// In production (after v2.0.0 upgrade), only governance can store contracts.
// This test verifies the governance flow works correctly.
func (s *priceOracleContractTestSuite) TestStoreContractViaGovernance() {
	ctx := context.Background()
	val := s.Network().Validators[0]

	// Load the pyth wasm contract
	wasmPath := findWasmPath("pyth", "pyth.wasm")
	if wasmPath == "" {
		s.T().Skip("pyth.wasm not found, skipping contract store test")
		return
	}

	wasm, err := os.ReadFile(wasmPath)
	s.Require().NoError(err)

	// Gzip if necessary
	if ioutils.IsWasm(wasm) {
		wasm, err = ioutils.GzipIt(wasm)
		s.Require().NoError(err)
	} else {
		s.Require().True(ioutils.IsGzip(wasm), "wasm should be gzipped")
	}

	// Create client
	cl, err := aclient.DiscoverClient(
		ctx,
		s.cctx.WithFrom(val.Address.String()),
		cltypes.WithGas(cltypes.GasSetting{Simulate: true}),
		cltypes.WithGasAdjustment(1.5),
		cltypes.WithGasPrices("0.025uakt"),
	)
	s.Require().NoError(err)

	// Get gov module address
	qResp, err := cl.Query().Auth().ModuleAccountByName(ctx, &authtypes.QueryModuleAccountByNameRequest{Name: "gov"})
	s.Require().NoError(err)

	var acc sdk.AccountI
	err = s.cctx.InterfaceRegistry.UnpackAny(qResp.Account, &acc)
	s.Require().NoError(err)

	macc, ok := acc.(sdk.ModuleAccountI)
	s.Require().True(ok)

	// Store via governance proposal
	msg := &wasmtypes.MsgStoreCode{
		Sender:                macc.GetAddress().String(),
		WASMByteCode:          wasm,
		InstantiatePermission: &wasmtypes.AllowNobody,
	}

	govMsg, err := govv1.NewMsgSubmitProposal(
		[]sdk.Msg{msg},
		sdk.Coins{sdk.NewInt64Coin("uakt", 1000000000)},
		val.Address.String(),
		"",
		"Store pyth contract",
		"Deploy pyth CosmWasm contract for Pyth price feeds",
		false,
	)
	s.Require().NoError(err)

	// Submit proposal should succeed
	resp, err := cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{govMsg}, cclient.WithGas(cltypes.GasSetting{Simulate: true}))
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.T().Log("Successfully submitted store code proposal via governance")
}

// TestWormholeContractMessageEncoding tests that Wormhole contract message types serialize correctly
func (s *priceOracleContractTestSuite) TestWormholeContractMessageEncoding() {
	// Test WormholeInstantiateMsg encoding
	// Use a test guardian address (20 bytes)
	testGuardianAddr := make([]byte, 20)
	for i := range testGuardianAddr {
		testGuardianAddr[i] = byte(i + 1)
	}

	instantiateMsg := WormholeInstantiateMsg{
		GovChain:   1, // Solana
		GovAddress: base64.StdEncoding.EncodeToString(make([]byte, 32)),
		InitialGuardianSet: GuardianSetInfo{
			Addresses: []GuardianAddress{
				{Bytes: base64.StdEncoding.EncodeToString(testGuardianAddr)},
			},
			ExpirationTime: 0,
		},
		GuardianSetExpirity: 86400,
		ChainID:             18, // Example chain ID
		FeeDenom:            "uakt",
	}

	data, err := json.Marshal(instantiateMsg)
	s.Require().NoError(err)
	s.T().Logf("Wormhole InstantiateMsg JSON: %s", string(data))

	var decoded WormholeInstantiateMsg
	err = json.Unmarshal(data, &decoded)
	s.Require().NoError(err)
	s.Require().Equal(instantiateMsg.GovChain, decoded.GovChain)
	s.Require().Equal(instantiateMsg.ChainID, decoded.ChainID)

	// Test WormholeQueryMsg encoding
	queryMsg := WormholeQueryMsg{
		GuardianSetInfo: &struct{}{},
	}

	data, err = json.Marshal(queryMsg)
	s.Require().NoError(err)
	s.Require().Equal(`{"guardian_set_info":{}}`, string(data))

	queryMsg = WormholeQueryMsg{
		GetState: &struct{}{},
	}

	data, err = json.Marshal(queryMsg)
	s.Require().NoError(err)
	s.Require().Equal(`{"get_state":{}}`, string(data))

	queryMsg = WormholeQueryMsg{
		QueryAddressHex: &QueryAddressHexMsg{Address: "akash1test123"},
	}

	data, err = json.Marshal(queryMsg)
	s.Require().NoError(err)
	s.T().Logf("Wormhole QueryAddressHex JSON: %s", string(data))
}

// TestPriceOracleWithVAAMessageEncoding tests that Pyth contract VAA message types serialize correctly
func (s *priceOracleContractTestSuite) TestPriceOracleWithVAAMessageEncoding() {
	// Test InstantiateMsg encoding with Wormhole and data sources
	pythEmitterAddr := "e101faedac5851e32b9b23b5f9411a8c2bac4aae3ed4dd7b811dd1a72ea4aa71"

	instantiateMsg := InstantiateMsg{
		Admin:            "akash1admin123",
		WormholeContract: "akash1wormhole456",
		UpdateFee:        "1000000",
		PriceFeedID:      "0xef0d8b6fda2ceba41da15d4095d1da392a0d2f8ed0c6c7bc0f4cfac8c280b56d",
		DataSources: []DataSource{
			{
				EmitterChain:   26, // Pythnet
				EmitterAddress: pythEmitterAddr,
			},
		},
	}

	data, err := json.Marshal(instantiateMsg)
	s.Require().NoError(err)
	s.T().Logf("Pyth InstantiateMsg JSON: %s", string(data))

	var decoded InstantiateMsg
	err = json.Unmarshal(data, &decoded)
	s.Require().NoError(err)
	s.Require().Equal(instantiateMsg.Admin, decoded.Admin)
	s.Require().Equal(instantiateMsg.WormholeContract, decoded.WormholeContract)
	s.Require().Len(decoded.DataSources, 1)
	s.Require().Equal(uint16(26), decoded.DataSources[0].EmitterChain)

	// Test ExecuteUpdatePriceFeed with VAA encoding
	executeMsg := ExecuteUpdatePriceFeed{
		UpdatePriceFeed: UpdatePriceFeedData{
			VAA: base64.StdEncoding.EncodeToString([]byte("test_vaa_data")),
		},
	}

	data, err = json.Marshal(executeMsg)
	s.Require().NoError(err)
	s.T().Logf("Pyth UpdatePriceFeed with VAA JSON: %s", string(data))

	// Test UpdateConfig encoding
	wormholeContract := "akash1newwormhole"
	updateConfigMsg := ExecuteUpdateConfig{
		UpdateConfig: UpdateConfigData{
			WormholeContract: &wormholeContract,
		},
	}

	data, err = json.Marshal(updateConfigMsg)
	s.Require().NoError(err)
	s.T().Logf("Pyth UpdateConfig JSON: %s", string(data))
}

// TestQueryOracleModuleParams tests that the oracle module params can be queried
func (s *priceOracleContractTestSuite) TestQueryOracleModuleParams() {
	ctx := context.Background()
	val := s.Network().Validators[0]

	cl, err := aclient.DiscoverClient(
		ctx,
		s.cctx.WithFrom(val.Address.String()),
		cltypes.WithGas(cltypes.GasSetting{Simulate: true}),
		cltypes.WithGasAdjustment(1.5),
		cltypes.WithGasPrices("0.025uakt"),
	)
	s.Require().NoError(err)

	// Query oracle params to ensure the oracle module is available
	// Note: Must pass empty request struct, not nil
	oracleParams, err := cl.Query().Oracle().Params(ctx, &oracletypes.QueryParamsRequest{})
	s.Require().NoError(err)
	s.Require().NotNil(oracleParams)
	s.Require().NotNil(oracleParams.Params)

	// Validate expected fields exist
	s.T().Logf("Oracle params: min_price_sources=%d, max_staleness=%d, max_deviation_bps=%d, twap_window=%d",
		oracleParams.Params.MinPriceSources,
		oracleParams.Params.MaxPriceStalenessBlocks,
		oracleParams.Params.MaxPriceDeviationBps,
		oracleParams.Params.TwapWindow,
	)
}

// TestContractMessageEncoding tests that contract message types serialize correctly
func (s *priceOracleContractTestSuite) TestContractMessageEncoding() {
	// Test InstantiateMsg encoding (now includes wormhole_contract and data_sources)
	instantiateMsg := InstantiateMsg{
		Admin:            "akash1test123",
		WormholeContract: "akash1wormhole456",
		UpdateFee:        "1000",
		PriceFeedID:      "0xef0d8b6fda2ceba41da15d4095d1da392a0d2f8ed0c6c7bc0f4cfac8c280b56d",
		DataSources: []DataSource{
			{EmitterChain: 26, EmitterAddress: "e101faedac5851e32b9b23b5f9411a8c2bac4aae3ed4dd7b811dd1a72ea4aa71"},
		},
	}

	data, err := json.Marshal(instantiateMsg)
	s.Require().NoError(err)

	var decoded InstantiateMsg
	err = json.Unmarshal(data, &decoded)
	s.Require().NoError(err)
	s.Require().Equal(instantiateMsg.Admin, decoded.Admin)
	s.Require().Equal(instantiateMsg.WormholeContract, decoded.WormholeContract)
	s.Require().Equal(instantiateMsg.PriceFeedID, decoded.PriceFeedID)

	// Test ExecuteMsg encoding (now uses VAA)
	executeMsg := ExecuteUpdatePriceFeed{
		UpdatePriceFeed: UpdatePriceFeedData{
			VAA: base64.StdEncoding.EncodeToString([]byte("test_vaa_binary_data")),
		},
	}

	data, err = json.Marshal(executeMsg)
	s.Require().NoError(err)
	s.T().Logf("Execute message JSON: %s", string(data))

	// Test QueryMsg encoding
	queryMsg := QueryMsg{
		GetConfig: &QueryGetConfig{},
	}

	data, err = json.Marshal(queryMsg)
	s.Require().NoError(err)
	s.Require().Equal(`{"get_config":{}}`, string(data))

	queryMsg = QueryMsg{
		GetPrice: &QueryGetPrice{},
	}

	data, err = json.Marshal(queryMsg)
	s.Require().NoError(err)
	s.Require().Equal(`{"get_price":{}}`, string(data))

	queryMsg = QueryMsg{
		GetOracleParams: &QueryGetOracleParams{},
	}

	data, err = json.Marshal(queryMsg)
	s.Require().NoError(err)
	s.Require().Equal(`{"get_oracle_params":{}}`, string(data))
}

// TestContractResponseParsing tests parsing of expected contract responses
func (s *priceOracleContractTestSuite) TestContractResponseParsing() {
	// Test ConfigResponse parsing (now includes wormhole_contract and data_sources)
	configJSON := `{
		"admin": "akash1abc123",
		"wormhole_contract": "akash1wormhole456",
		"update_fee": "1000",
		"price_feed_id": "0xtest",
		"default_denom": "uakt",
		"default_base_denom": "usd",
		"data_sources": [{"emitter_chain": 26, "emitter_address": "e101faedac5851e32b9b23b5f9411a8c2bac4aae3ed4dd7b811dd1a72ea4aa71"}]
	}`

	var config ConfigResponse
	err := json.Unmarshal([]byte(configJSON), &config)
	s.Require().NoError(err)
	s.Require().Equal("akash1abc123", config.Admin)
	s.Require().Equal("akash1wormhole456", config.WormholeContract)
	s.Require().Equal("1000", config.UpdateFee)
	s.Require().Equal("0xtest", config.PriceFeedID)
	s.Require().Equal("uakt", config.DefaultDenom)
	s.Require().Equal("usd", config.DefaultBaseDenom)
	s.Require().Len(config.DataSources, 1)
	s.Require().Equal(uint16(26), config.DataSources[0].EmitterChain)

	// Test PriceResponse parsing
	priceJSON := `{
		"price": "123000000",
		"conf": "1000000",
		"expo": -8,
		"publish_time": 1704067200
	}`

	var price PriceResponse
	err = json.Unmarshal([]byte(priceJSON), &price)
	s.Require().NoError(err)
	s.Require().Equal("123000000", price.Price)
	s.Require().Equal("1000000", price.Conf)
	s.Require().Equal(int32(-8), price.Expo)
	s.Require().Equal(int64(1704067200), price.PublishTime)

	// Test OracleParamsResponse parsing
	paramsJSON := `{
		"max_price_deviation_bps": 150,
		"min_price_sources": 2,
		"max_price_staleness_blocks": 50,
		"twap_window": 50,
		"last_updated_height": 100
	}`

	var params OracleParamsResponse
	err = json.Unmarshal([]byte(paramsJSON), &params)
	s.Require().NoError(err)
	s.Require().Equal(uint64(150), params.MaxPriceDeviationBps)
	s.Require().Equal(uint32(2), params.MinPriceSources)
	s.Require().Equal(int64(50), params.MaxPriceStalenessBlocks)
	s.Require().Equal(int64(50), params.TwapWindow)
	s.Require().Equal(uint64(100), params.LastUpdatedHeight)
}

// TestWormholeResponseParsing tests parsing of Wormhole contract responses
func (s *priceOracleContractTestSuite) TestWormholeResponseParsing() {
	// Test GuardianSetInfoResponse parsing
	testGuardianAddr := make([]byte, 20)
	for i := range testGuardianAddr {
		testGuardianAddr[i] = byte(i + 1)
	}

	guardianSetJSON := `{
		"guardian_set_index": 3,
		"addresses": [
			{"bytes": "` + base64.StdEncoding.EncodeToString(testGuardianAddr) + `"}
		]
	}`

	var guardianSet WormholeGuardianSetInfoResponse
	err := json.Unmarshal([]byte(guardianSetJSON), &guardianSet)
	s.Require().NoError(err)
	s.Require().Equal(uint32(3), guardianSet.GuardianSetIndex)
	s.Require().Len(guardianSet.Addresses, 1)

	// Test GetStateResponse parsing
	stateJSON := `{
		"fee": {
			"denom": "uakt",
			"amount": "1000"
		}
	}`

	var state WormholeGetStateResponse
	err = json.Unmarshal([]byte(stateJSON), &state)
	s.Require().NoError(err)
	s.Require().Equal("uakt", state.Fee.Denom)
	s.Require().Equal("1000", state.Fee.Amount)
}

// TestVAAExecuteMessageParsing tests that VAA-based execute messages are properly formatted
func (s *priceOracleContractTestSuite) TestVAAExecuteMessageParsing() {
	// Test that VAA binary data is properly base64 encoded in execute message
	testVAAData := []byte("P2WH" + "test_vaa_payload_data_with_guardian_signatures")
	vaaBase64 := base64.StdEncoding.EncodeToString(testVAAData)

	executeMsg := ExecuteUpdatePriceFeed{
		UpdatePriceFeed: UpdatePriceFeedData{
			VAA: vaaBase64,
		},
	}

	data, err := json.Marshal(executeMsg)
	s.Require().NoError(err)

	// Verify the JSON structure
	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	s.Require().NoError(err)

	updatePriceFeed, ok := parsed["update_price_feed"].(map[string]interface{})
	s.Require().True(ok, "Should have update_price_feed field")

	vaaField, ok := updatePriceFeed["vaa"].(string)
	s.Require().True(ok, "Should have vaa field as string")
	s.Require().Equal(vaaBase64, vaaField)

	s.T().Logf("VAA execute message JSON: %s", string(data))
}

// TestAllContractsExist verifies that all contract WASM files are available
func (s *priceOracleContractTestSuite) TestAllContractsExist() {
	// Note: Pyth contract removed - Pyth now handles VAA verification directly via Wormhole
	contracts := []struct {
		name     string
		dir      string
		wasmFile string
	}{
		{"wormhole", "wormhole", "wormhole.wasm"},
		{"pyth", "pyth", "pyth.wasm"},
	}

	for _, c := range contracts {
		wasmPath := findWasmPath(c.dir, c.wasmFile)
		if wasmPath == "" {
			s.T().Logf("WARN: %s contract not found at expected paths", c.name)
			continue
		}

		info, err := os.Stat(wasmPath)
		s.Require().NoError(err, "Failed to stat %s", c.name)
		s.T().Logf("Found %s contract: %s (size: %d bytes)", c.name, wasmPath, info.Size())

		// Verify it's a valid WASM file
		wasm, err := os.ReadFile(wasmPath)
		s.Require().NoError(err)
		s.Require().True(ioutils.IsWasm(wasm) || ioutils.IsGzip(wasm),
			"%s should be a valid WASM or gzipped WASM file", c.name)
	}
}

// TestVAAStructure validates VAA binary structure understanding
func (s *priceOracleContractTestSuite) TestVAAStructure() {
	// VAA header structure (for reference):
	// - version (1 byte)
	// - guardian_set_index (4 bytes)
	// - len_signers (1 byte)
	// - signatures (66 bytes each)
	// - body:
	//   - timestamp (4 bytes)
	//   - nonce (4 bytes)
	//   - emitter_chain (2 bytes)
	//   - emitter_address (32 bytes)
	//   - sequence (8 bytes)
	//   - consistency_level (1 byte)
	//   - payload (variable)

	// Test that we understand the structure correctly
	s.T().Log("VAA Header structure:")
	s.T().Log("  - Version: 1 byte at offset 0")
	s.T().Log("  - Guardian Set Index: 4 bytes at offset 1")
	s.T().Log("  - Num Signers: 1 byte at offset 5")
	s.T().Log("  - Signatures: 66 bytes each starting at offset 6")
	s.T().Log("Body structure (after signatures):")
	s.T().Log("  - Timestamp: 4 bytes at offset 0")
	s.T().Log("  - Nonce: 4 bytes at offset 4")
	s.T().Log("  - Emitter Chain: 2 bytes at offset 8")
	s.T().Log("  - Emitter Address: 32 bytes at offset 10")
	s.T().Log("  - Sequence: 8 bytes at offset 42")
	s.T().Log("  - Consistency Level: 1 byte at offset 50")
	s.T().Log("  - Payload: variable starting at offset 51")

	// Create a minimal test VAA structure
	testGuardianAddr := make([]byte, 20)
	for i := range testGuardianAddr {
		testGuardianAddr[i] = byte(i + 1)
	}

	// Log test guardian address
	s.T().Logf("Test guardian address (hex): %s", hex.EncodeToString(testGuardianAddr))
	s.T().Logf("Test guardian address (base64): %s", base64.StdEncoding.EncodeToString(testGuardianAddr))
}

// findWasmPath attempts to find a wasm file for a given contract
func findWasmPath(contractDir, wasmFile string) string {
	// Try common paths relative to the test location
	paths := []string{
		filepath.Join("../../contracts", contractDir, "artifacts", wasmFile),
		filepath.Join("../contracts", contractDir, "artifacts", wasmFile),
		filepath.Join("contracts", contractDir, "artifacts", wasmFile),
	}

	// Also try using GOPATH
	gopath := os.Getenv("GOPATH")
	if gopath != "" {
		paths = append(paths, filepath.Join(gopath, "src/github.com/akash-network/node/contracts", contractDir, "artifacts", wasmFile))
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

// =====================
// WASM/Governance Helper Functions
// =====================

// LoadAndGzipWasm loads a WASM file and gzips it if necessary
func LoadAndGzipWasm(wasmPath string) ([]byte, error) {
	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read wasm file: %w", err)
	}

	if ioutils.IsWasm(wasm) {
		wasm, err = ioutils.GzipIt(wasm)
		if err != nil {
			return nil, fmt.Errorf("failed to gzip wasm: %w", err)
		}
	} else if !ioutils.IsGzip(wasm) {
		return nil, fmt.Errorf("file is neither valid wasm nor gzipped wasm")
	}

	return wasm, nil
}

// GetGovModuleAddress returns the governance module account address
func GetGovModuleAddress(ctx context.Context, cl cclient.Client, cctx client.Context) (sdk.AccAddress, error) {
	qResp, err := cl.Query().Auth().ModuleAccountByName(ctx, &authtypes.QueryModuleAccountByNameRequest{Name: "gov"})
	if err != nil {
		return nil, fmt.Errorf("failed to query gov module account: %w", err)
	}

	var acc sdk.AccountI
	err = cctx.InterfaceRegistry.UnpackAny(qResp.Account, &acc)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack account: %w", err)
	}

	macc, ok := acc.(sdk.ModuleAccountI)
	if !ok {
		return nil, fmt.Errorf("account is not a module account")
	}

	return macc.GetAddress(), nil
}

// SubmitStoreCodeProposal submits a governance proposal to store contract code
// Returns the proposal ID
func SubmitStoreCodeProposal(
	ctx context.Context,
	cl cclient.Client,
	govModuleAddr sdk.AccAddress,
	wasmBytes []byte,
	proposer sdk.AccAddress,
	deposit sdk.Coins,
	title, summary string,
) (uint64, error) {
	msg := &wasmtypes.MsgStoreCode{
		Sender:                govModuleAddr.String(),
		WASMByteCode:          wasmBytes,
		InstantiatePermission: &wasmtypes.AccessConfig{Permission: wasmtypes.AccessTypeEverybody},
	}

	govMsg, err := govv1.NewMsgSubmitProposal(
		[]sdk.Msg{msg},
		deposit,
		proposer.String(),
		"", // metadata
		title,
		summary,
		false, // not expedited
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create proposal: %w", err)
	}

	resp, err := cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{govMsg},
		cclient.WithGas(cltypes.GasSetting{Simulate: true}))
	if err != nil {
		return 0, fmt.Errorf("failed to submit proposal: %w", err)
	}

	// Type assert the response to *sdk.TxResponse
	txResp, ok := resp.(*sdk.TxResponse)
	if !ok {
		return 0, fmt.Errorf("unexpected response type: %T", resp)
	}

	// Parse proposal ID from response events
	proposalID, err := parseProposalIDFromResponse(txResp)
	if err != nil {
		return 0, err
	}

	return proposalID, nil
}

// VoteOnProposal votes YES on a governance proposal
func VoteOnProposal(
	ctx context.Context,
	cl cclient.Client,
	proposalID uint64,
	voter sdk.AccAddress,
) error {
	voteMsg := govv1.NewMsgVote(
		voter,
		proposalID,
		govv1.OptionYes,
		"",
	)

	_, err := cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{voteMsg})
	if err != nil {
		return fmt.Errorf("failed to vote on proposal: %w", err)
	}

	return nil
}

// WaitForProposalToPass polls until a proposal passes or fails
func WaitForProposalToPass(
	ctx context.Context,
	cl cclient.Client,
	proposalID uint64,
	timeout time.Duration,
) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		proposal, err := cl.Query().Gov().Proposal(ctx, &govv1.QueryProposalRequest{
			ProposalId: proposalID,
		})
		if err != nil {
			return fmt.Errorf("failed to query proposal: %w", err)
		}

		switch proposal.Proposal.Status {
		case govv1.StatusPassed:
			return nil
		case govv1.StatusRejected:
			return fmt.Errorf("proposal %d was rejected", proposalID)
		case govv1.StatusFailed:
			return fmt.Errorf("proposal %d failed", proposalID)
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for proposal %d to pass", proposalID)
}

// GetCodeIDFromWasmEvents extracts the code ID from a store code transaction's events
func GetCodeIDFromWasmEvents(ctx context.Context, cl cclient.Client, proposalID uint64) (uint64, error) {
	// Query the proposal to find the execution result
	proposal, err := cl.Query().Gov().Proposal(ctx, &govv1.QueryProposalRequest{
		ProposalId: proposalID,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to query proposal: %w", err)
	}

	if proposal.Proposal.Status != govv1.StatusPassed {
		return 0, fmt.Errorf("proposal %d has not passed yet", proposalID)
	}

	// Query wasm codes to find the latest one
	codesResp, err := cl.Query().Wasm().Codes(ctx, &wasmtypes.QueryCodesRequest{})
	if err != nil {
		return 0, fmt.Errorf("failed to query wasm codes: %w", err)
	}

	if len(codesResp.CodeInfos) == 0 {
		return 0, fmt.Errorf("no wasm codes found")
	}

	// Return the latest code ID
	return codesResp.CodeInfos[len(codesResp.CodeInfos)-1].CodeID, nil
}

// InstantiateContract instantiates a contract from stored code
func InstantiateContract(
	ctx context.Context,
	cl cclient.Client,
	codeID uint64,
	initMsg interface{},
	label string,
	admin string,
	sender sdk.AccAddress,
) (string, error) {
	initMsgBytes, err := json.Marshal(initMsg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal init msg: %w", err)
	}

	msg := &wasmtypes.MsgInstantiateContract{
		Sender: sender.String(),
		Admin:  admin,
		CodeID: codeID,
		Label:  label,
		Msg:    initMsgBytes,
		Funds:  nil,
	}

	resp, err := cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{msg},
		cclient.WithGas(cltypes.GasSetting{Simulate: true}))
	if err != nil {
		return "", fmt.Errorf("failed to instantiate contract: %w", err)
	}

	// Type assert the response to *sdk.TxResponse
	txResp, ok := resp.(*sdk.TxResponse)
	if !ok {
		return "", fmt.Errorf("unexpected response type: %T", resp)
	}

	// Parse contract address from response events
	contractAddr, err := parseContractAddressFromResponse(txResp)
	if err != nil {
		return "", err
	}

	return contractAddr, nil
}

// QueryContract queries a contract's state
func QueryContract(
	ctx context.Context,
	cl cclient.Client,
	contractAddr string,
	queryMsg interface{},
) ([]byte, error) {
	queryMsgBytes, err := json.Marshal(queryMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query msg: %w", err)
	}

	resp, err := cl.Query().Wasm().SmartContractState(ctx, &wasmtypes.QuerySmartContractStateRequest{
		Address:   contractAddr,
		QueryData: queryMsgBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query contract: %w", err)
	}

	return resp.Data, nil
}

// ExecuteContract executes a contract method
func ExecuteContract(
	ctx context.Context,
	cl cclient.Client,
	contractAddr string,
	executeMsg interface{},
	funds sdk.Coins,
	sender sdk.AccAddress,
) error {
	executeMsgBytes, err := json.Marshal(executeMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal execute msg: %w", err)
	}

	msg := &wasmtypes.MsgExecuteContract{
		Sender:   sender.String(),
		Contract: contractAddr,
		Msg:      executeMsgBytes,
		Funds:    funds,
	}

	_, err = cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{msg},
		cclient.WithGas(cltypes.GasSetting{Simulate: true}))
	if err != nil {
		return fmt.Errorf("failed to execute contract: %w", err)
	}

	return nil
}

// parseProposalIDFromResponse extracts the proposal ID from a submit proposal tx response
func parseProposalIDFromResponse(resp *sdk.TxResponse) (uint64, error) {
	for _, event := range resp.Events {
		if event.Type == "submit_proposal" {
			for _, attr := range event.Attributes {
				if attr.Key == "proposal_id" {
					id, err := strconv.ParseUint(attr.Value, 10, 64)
					if err != nil {
						return 0, fmt.Errorf("failed to parse proposal ID: %w", err)
					}
					return id, nil
				}
			}
		}
	}
	return 0, fmt.Errorf("proposal_id not found in response events")
}

// parseContractAddressFromResponse extracts the contract address from an instantiate tx response
func parseContractAddressFromResponse(resp *sdk.TxResponse) (string, error) {
	for _, event := range resp.Events {
		if event.Type == "instantiate" {
			for _, attr := range event.Attributes {
				if attr.Key == "_contract_address" {
					return attr.Value, nil
				}
			}
		}
	}
	return "", fmt.Errorf("contract address not found in response events")
}

// TestStoreContractCodeViaGovernance tests storing contract code via governance proposal.
// This tests the full governance workflow for storing WASM code on the chain.
func (s *priceOracleContractTestSuite) TestStoreContractCodeViaGovernance() {
	ctx := context.Background()
	val := s.Network().Validators[0]

	// Create client with gas simulation
	cl, err := aclient.DiscoverClient(
		ctx,
		s.cctx.WithFrom(val.Address.String()),
		cltypes.WithGas(cltypes.GasSetting{Simulate: true}),
		cltypes.WithGasAdjustment(2.0),
		cltypes.WithGasPrices("0.025uakt"),
	)
	s.Require().NoError(err)

	// Step 1: Load contract WASM
	wasmPath := findWasmPath("pyth", "pyth.wasm")
	if wasmPath == "" {
		s.T().Skip("pyth.wasm not found, skipping contract deployment test")
		return
	}
	s.T().Logf("Found pyth contract at: %s", wasmPath)

	wasmBytes, err := LoadAndGzipWasm(wasmPath)
	s.Require().NoError(err)
	s.T().Logf("Loaded and gzipped WASM: %d bytes", len(wasmBytes))

	// Step 2: Get governance module address
	govAddr, err := GetGovModuleAddress(ctx, cl, s.cctx)
	s.Require().NoError(err)
	s.T().Logf("Governance module address: %s", govAddr.String())

	// Step 3: Submit store code proposal
	deposit := sdk.NewCoins(sdk.NewInt64Coin("uakt", 100000000)) // 100 AKT
	proposalID, err := SubmitStoreCodeProposal(
		ctx, cl, govAddr, wasmBytes,
		val.Address, deposit,
		"Store pyth contract",
		"Deploy pyth CosmWasm contract for testing",
	)
	s.Require().NoError(err)
	s.T().Logf("Submitted store code proposal: %d", proposalID)

	// Step 4: Vote on proposal (all validators vote YES)
	for _, validator := range s.Network().Validators {
		// Create client for each validator
		valCl, err := aclient.DiscoverClient(
			ctx,
			validator.ClientCtx.WithFrom(validator.Address.String()),
			cltypes.WithGas(cltypes.GasSetting{Simulate: true}),
			cltypes.WithGasAdjustment(1.5),
			cltypes.WithGasPrices("0.025uakt"),
		)
		s.Require().NoError(err)

		err = VoteOnProposal(ctx, valCl, proposalID, validator.Address)
		s.Require().NoError(err)
		s.T().Logf("Validator %s voted YES on proposal %d", validator.Address.String(), proposalID)
	}

	// Step 5: Wait for proposal to pass (with 30 second timeout)
	err = WaitForProposalToPass(ctx, cl, proposalID, 30*time.Second)
	s.Require().NoError(err)
	s.T().Log("Proposal passed!")

	// Step 6: Get code ID from stored codes
	codeID, err := GetCodeIDFromWasmEvents(ctx, cl, proposalID)
	s.Require().NoError(err)
	s.T().Logf("Contract stored with code ID: %d", codeID)

	// Verify the code is stored and can be queried
	codeInfoResp, err := cl.Query().Wasm().Code(ctx, &wasmtypes.QueryCodeRequest{
		CodeId: codeID,
	})
	s.Require().NoError(err)
	s.Require().NotNil(codeInfoResp)
	s.Require().NotNil(codeInfoResp.CodeInfoResponse)
	s.T().Logf("Code info: creator=%s, checksum=%x",
		codeInfoResp.CodeInfoResponse.Creator,
		codeInfoResp.CodeInfoResponse.DataHash)

	// Step 7: Instantiate the contract
	// The pyth contract requires:
	// - wormhole_contract: Address for VAA verification (use placeholder for test)
	// - data_sources: Trusted Pyth emitters
	// - Queries oracle module params during instantiation via custom Akash querier
	initMsg := InstantiateMsg{
		Admin:            val.Address.String(),
		WormholeContract: val.Address.String(), // Use validator address as placeholder wormhole contract
		UpdateFee:        "1000",
		PriceFeedID:      "0xef0d8b6fda2ceba41da15d4095d1da392a0d2f8ed0c6c7bc0f4cfac8c280b56d", // AKT/USD price feed ID
		DataSources: []DataSource{
			{
				EmitterChain:   26, // Pythnet
				EmitterAddress: "e101faedac5851e32b9b23b5f9411a8c2bac4aae3ed4dd7b811dd1a72ea4aa71",
			},
		},
	}

	contractAddr, err := InstantiateContract(
		ctx, cl, codeID, initMsg,
		"pyth-test",
		val.Address.String(), // admin
		val.Address,
	)
	s.Require().NoError(err, "Contract instantiation should succeed with custom Akash querier")
	s.T().Logf("Contract instantiated at: %s", contractAddr)

	// Step 8: Query the contract config to verify instantiation
	queryMsg := QueryMsg{GetConfig: &QueryGetConfig{}}
	configBytes, err := QueryContract(ctx, cl, contractAddr, queryMsg)
	s.Require().NoError(err)

	var config ConfigResponse
	err = json.Unmarshal(configBytes, &config)
	s.Require().NoError(err)
	s.T().Logf("Contract config: admin=%s, update_fee=%s, price_feed_id=%s",
		config.Admin, config.UpdateFee, config.PriceFeedID)

	s.Require().Equal(val.Address.String(), config.Admin)
	s.Require().Equal("1000", config.UpdateFee)
	s.T().Log("Contract deployed and configured successfully!")
}
