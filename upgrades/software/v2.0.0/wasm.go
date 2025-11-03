package v2_0_0

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"pkg.akt.dev/go/sdkutil"
)

const (
	// Pythnet emitter for Pyth price feeds
	pythnetEmitterChain   = 26
	pythnetEmitterAddress = "e101faedac5851e32b9b23b5f9411a8c2bac4aae3ed4dd7b811dd1a72ea4aa71"
	aktPriceFeedID        = "0x4ea5bb4d2f5900cc2e97ba534240950740b4d3b89fe712a94a7304fd2fd92702"
)

type wormholeGuardianAddress struct {
	Bytes string `json:"bytes"` // base64-encoded 20-byte guardian address
}

type wormholeGuardianSet struct {
	Addresses      []wormholeGuardianAddress `json:"addresses"`
	ExpirationTime uint64                    `json:"expiration_time"`
}

// wormholeInstantiateMsg matches contracts/wormhole/src/msg.rs::InstantiateMsg
type wormholeInstantiateMsg struct {
	GovChain            uint16              `json:"gov_chain"`
	GovAddress          string              `json:"gov_address"`
	ChainID             uint16              `json:"chain_id"`
	FeeDenom            string              `json:"fee_denom"`
	InitialGuardianSet  wormholeGuardianSet `json:"initial_guardian_set"`
	GuardianSetIndex    uint32              `json:"guardian_set_index"`
	GuardianSetExpirity uint64              `json:"guardian_set_expirity"`
}

// pythInstantiateMsg matches contracts/pyth/src/msg.rs::InstantiateMsg
type pythInstantiateMsg struct {
	Admin            string          `json:"admin"`
	WormholeContract string          `json:"wormhole_contract"`
	UpdateFee        string          `json:"update_fee"`
	PriceFeedID      string          `json:"price_feed_id"`
	DataSources      []dataSourceMsg `json:"data_sources"`
}

// dataSourceMsg matches contracts/pyth/src/msg.rs::DataSourceMsg
type dataSourceMsg struct {
	EmitterChain   uint16 `json:"emitter_chain"`
	EmitterAddress string `json:"emitter_address"`
}

func (up *upgrade) instantiateOracleContracts(ctx context.Context) (string, error) {
	msgServer := wasmkeeper.NewMsgServerImpl(up.Keepers.Cosmos.Wasm)
	govAddr := up.Keepers.Cosmos.Wasm.GetAuthority()

	// Store and instantiate Wormhole contract
	wormholeAddr, err := up.storeAndInstantiateWormhole(ctx, msgServer, govAddr, govAddr)
	if err != nil {
		return "", fmt.Errorf("wormhole: %w", err)
	}

	up.log.Info("wormhole contract instantiated", "address", wormholeAddr)

	// Store and instantiate Pyth contract (depends on wormhole address)
	pythAddr, err := up.storeAndInstantiatePyth(ctx, msgServer, govAddr, govAddr, wormholeAddr)
	if err != nil {
		return "", fmt.Errorf("pyth: %w", err)
	}

	up.log.Info("pyth contract instantiated", "address", pythAddr)

	return pythAddr, nil
}

func (up *upgrade) storeAndInstantiateWormhole(
	ctx context.Context,
	msgServer wasmtypes.MsgServer,
	govAddr, adminAddr string,
) (string, error) {
	codeID, err := storeContract(ctx, msgServer, govAddr, wormholeContract)
	if err != nil {
		return "", fmt.Errorf("store code: %w", err)
	}

	// Wormhole mainnet governance emitter address: value 4 left-padded to 32 bytes.
	// Source: https://github.com/wormhole-foundation/wormhole/blob/85af3ce56e000fae61c371de69b2e5e41bebe412/wormchain/contracts/tools/deploy_wormchain.ts#L61
	govEmitter := make([]byte, 32)
	govEmitter[31] = 0x04

	// Wormhole Mainnet Guardian Set 5 (19 guardians, index 4)
	// Source: https://github.com/wormhole-foundation/wormhole/blob/main/guardianset/mainnetv2/v5.prototxt
	guardianHexAddresses := []string{
		"5893B5A76c3f739645648885bDCcC06cd70a3Cd3",
		"fF6CB952589BDE862c25Ef4392132fb9D4A42157",
		"114De8460193bdf3A2fCf81f86a09765F4762fD1",
		"107A0086b32d7A0977926A205131d8731D39cbEB",
		"8C82B2fd82FaeD2711d59AF0F2499D16e726f6b2",
		"11b39756C042441BE6D8650b69b54EbE715E2343",
		"938f104AEb5581293216ce97d771e0CB721221B1",
		"15e7cAF07C4e3DC8e7C469f92C8Cd88FB8005a20",
		"74a3bf913953D695260D88BC1aA25A4eeE363ef0",
		"000aC0076727b35FBea2dAc28fEE5cCB0fEA768e",
		"AF45Ced136b9D9e24903464AE889F5C8a723FC14",
		"f93124b7c738843CBB89E864c862c38cddCccF95",
		"D2CC37A4dc036a8D232b48f62cDD4731412f4890",
		"DA798F6896A3331F64b48c12D1D57Fd9cbe70811",
		"D1F64e26238811de5553C40f64af41eE1B6057Cc",
		"43ac8f567A31e7850Da532B361988Bfe0d3ae11b",
		"178e21ad2E77AE06711549CFBB1f9c7a9d8096e8",
		"5E1487F35515d02A92753504a8D75471b9f49EdB",
		"6FbEBc898F403E4773E95feB15E80C9A99c8348d",
	}

	guardianAddresses := make([]wormholeGuardianAddress, len(guardianHexAddresses))
	for i, hexAddr := range guardianHexAddresses {
		addrBytes, err := hex.DecodeString(hexAddr)
		if err != nil {
			return "", fmt.Errorf("decode guardian address %s: %w", hexAddr, err)
		}
		guardianAddresses[i] = wormholeGuardianAddress{
			Bytes: base64.StdEncoding.EncodeToString(addrBytes),
		}
	}

	initMsg := wormholeInstantiateMsg{
		GovChain:   1, // Solana (Wormhole governance chain)
		GovAddress: base64.StdEncoding.EncodeToString(govEmitter),
		ChainID:    26, // Pythnet chain-id
		FeeDenom:   sdkutil.DenomUakt,
		InitialGuardianSet: wormholeGuardianSet{
			Addresses:      guardianAddresses,
			ExpirationTime: 0,
		},
		GuardianSetIndex:    5, // Guardian Set 5
		GuardianSetExpirity: 86400,
	}

	initMsgBz, err := json.Marshal(initMsg)
	if err != nil {
		return "", fmt.Errorf("marshal init msg: %w", err)
	}

	resp, err := msgServer.InstantiateContract(ctx, &wasmtypes.MsgInstantiateContract{
		Sender: govAddr,
		Admin:  adminAddr,
		CodeID: codeID,
		Label:  "wormhole",
		Msg:    initMsgBz,
	})
	if err != nil {
		return "", fmt.Errorf("instantiate: %w", err)
	}

	return resp.Address, nil
}

func (up *upgrade) storeAndInstantiatePyth(
	ctx context.Context,
	msgServer wasmtypes.MsgServer,
	govAddr string,
	adminAddr string,
	wormholeAddr string,
) (string, error) {
	codeID, err := storeContract(ctx, msgServer, govAddr, pythContract)
	if err != nil {
		return "", fmt.Errorf("store code: %w", err)
	}

	initMsg := pythInstantiateMsg{
		Admin:            govAddr,
		WormholeContract: wormholeAddr,
		UpdateFee:        "1000",
		PriceFeedID:      aktPriceFeedID,
		DataSources: []dataSourceMsg{
			{
				EmitterChain:   pythnetEmitterChain,
				EmitterAddress: pythnetEmitterAddress,
			},
		},
	}

	initMsgBz, err := json.Marshal(initMsg)
	if err != nil {
		return "", fmt.Errorf("marshal init msg: %w", err)
	}

	resp, err := msgServer.InstantiateContract(ctx, &wasmtypes.MsgInstantiateContract{
		Sender: govAddr,
		Admin:  adminAddr,
		CodeID: codeID,
		Label:  "pyth",
		Msg:    initMsgBz,
	})
	if err != nil {
		return "", fmt.Errorf("instantiate: %w", err)
	}

	return resp.Address, nil
}

func storeContract(
	ctx context.Context,
	msgServer wasmtypes.MsgServer,
	sender string,
	wasmBz []byte,
) (uint64, error) {
	// The MsgStoreCode handler accepts both raw wasm and gzipped wasm.
	// Raw wasm is fine — the keeper will handle compression internally.
	resp, err := msgServer.StoreCode(ctx, &wasmtypes.MsgStoreCode{
		Sender:                sender,
		WASMByteCode:          wasmBz,
		InstantiatePermission: &wasmtypes.AllowNobody,
	})
	if err != nil {
		return 0, fmt.Errorf("store code: %w", err)
	}

	return resp.CodeID, nil
}
