package grpcsuite

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/CosmWasm/wasmd/x/wasm/ioutils"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"
)

type cosmwasmPack struct{}

func (cosmwasmPack) Name() string { return "cosmwasm" }

func (cosmwasmPack) Available(d *Discovery) bool { return d.HasModule("cosmwasm.wasm.v1") }

func (cosmwasmPack) Run(s *Suite) {
	q := wasmtypes.NewQueryClient(s.Conn)
	params, err := q.Params(s.Ctx, &wasmtypes.QueryParamsRequest{})
	require.NoError(s.T, err, "wasm Params")
	require.NotNil(s.T, params, "wasm Params response")

	actor := s.FundAccountDefault("cosmwasm")
	admin := s.FundAccountDefault("cosmwasm-admin")
	wasm := loadHackatomWasm(s)
	authority := s.GovAuthority()
	initMsg := hackatomInstantiateMsg(actor, admin)
	emptyMsg := wasmtypes.RawContractMessage(`{}`)
	allowEverybody := wasmtypes.AllowEverybody
	createErrs := []string{
		"can not create code",
		"code upload",
		"create wasm contract failed",
		"permission",
		"unauthorized",
	}
	contractErrs := []string{
		"Error calling the VM",
		"execute wasm contract failed",
		"instantiate wasm contract failed",
		"migrate wasm contract failed",
		"not found",
		"no such code",
		"no such contract",
		"permission",
		"unauthorized",
	}

	s.PassGovProposal("grpcsuite: store wasm code",
		&wasmtypes.MsgStoreCode{
			Sender:                authority,
			WASMByteCode:          wasm,
			InstantiatePermission: &allowEverybody,
		})

	codeID := latestWasmCodeID(s, q)
	code, err := q.Code(s.Ctx, &wasmtypes.QueryCodeRequest{CodeId: codeID})
	require.NoError(s.T, err, "wasm Code")
	require.NotEmpty(s.T, code.Data, "stored wasm code should be queryable")

	s.BroadcastTolerant("cosmwasm", createErrs, &wasmtypes.MsgStoreCode{
		Sender:                actor.String(),
		WASMByteCode:          []byte("not wasm"),
		InstantiatePermission: &allowEverybody,
	})

	s.BroadcastOK("cosmwasm", &wasmtypes.MsgInstantiateContract{
		Sender: actor.String(),
		Admin:  actor.String(),
		CodeID: codeID,
		Label:  "grpcsuite-hackatom",
		Msg:    initMsg,
		Funds:  sdk.NewCoins(sdk.NewInt64Coin(s.Env.BondDenom, 1_000_000)),
	})
	contract := latestContractForCode(s, q, codeID)
	_, err = q.ContractInfo(s.Ctx, &wasmtypes.QueryContractInfoRequest{Address: contract})
	require.NoError(s.T, err, "wasm ContractInfo")

	s.BroadcastOK("cosmwasm", &wasmtypes.MsgInstantiateContract2{
		Sender: actor.String(),
		Admin:  actor.String(),
		CodeID: codeID,
		Label:  "grpcsuite-hackatom-2",
		Msg:    initMsg,
		Salt:   []byte("grpcsuite-hackatom-2"),
		FixMsg: true,
	})

	s.BroadcastOK("cosmwasm", &wasmtypes.MsgExecuteContract{
		Sender:   actor.String(),
		Contract: contract,
		Msg:      wasmtypes.RawContractMessage(`{"release":{}}`),
	})
	s.BroadcastTolerant("cosmwasm", contractErrs, &wasmtypes.MsgMigrateContract{
		Sender:   actor.String(),
		Contract: contract,
		CodeID:   codeID,
		Msg:      emptyMsg,
	})
	s.BroadcastOK("cosmwasm", &wasmtypes.MsgUpdateContractLabel{
		Sender:   actor.String(),
		Contract: contract,
		NewLabel: "grpcsuite-hackatom-updated",
	})
	s.BroadcastOK("cosmwasm", &wasmtypes.MsgUpdateAdmin{
		Sender:   actor.String(),
		Contract: contract,
		NewAdmin: admin.String(),
	})
	s.BroadcastOK("cosmwasm-admin", &wasmtypes.MsgClearAdmin{
		Sender:   admin.String(),
		Contract: contract,
	})

	s.PassGovProposal("grpcsuite: update wasm code config",
		&wasmtypes.MsgUpdateInstantiateConfig{
			Sender:                   authority,
			CodeID:                   codeID,
			NewInstantiatePermission: &allowEverybody,
		},
		&wasmtypes.MsgPinCodes{Authority: authority, CodeIDs: []uint64{codeID}},
		&wasmtypes.MsgUnpinCodes{Authority: authority, CodeIDs: []uint64{codeID}},
	)

	s.BroadcastExpectErr("cosmwasm", &wasmtypes.MsgUpdateParams{Authority: actor.String(), Params: params.Params})
	s.BroadcastExpectErr("cosmwasm", &wasmtypes.MsgSudoContract{
		Authority: actor.String(),
		Contract:  contract,
		Msg:       emptyMsg,
	})
	s.BroadcastExpectErr("cosmwasm", &wasmtypes.MsgAddCodeUploadParamsAddresses{
		Authority: actor.String(),
		Addresses: []string{actor.String()},
	})
	s.BroadcastExpectErr("cosmwasm", &wasmtypes.MsgRemoveCodeUploadParamsAddresses{
		Authority: actor.String(),
		Addresses: []string{actor.String()},
	})
	s.BroadcastTolerant("cosmwasm", contractErrs, &wasmtypes.MsgStoreAndInstantiateContract{
		Authority:             actor.String(),
		WASMByteCode:          wasm,
		InstantiatePermission: &allowEverybody,
		Admin:                 actor.String(),
		Label:                 "grpcsuite-hackatom-store-instantiate",
		Msg:                   initMsg,
	})
	s.BroadcastTolerant("cosmwasm", contractErrs, &wasmtypes.MsgStoreAndMigrateContract{
		Authority:             actor.String(),
		WASMByteCode:          wasm,
		InstantiatePermission: &allowEverybody,
		Contract:              contract,
		Msg:                   emptyMsg,
	})

	s.logf("cosmwasm complete (store, instantiate, execute, admin, pin/unpin, params, negatives)")
}

func loadHackatomWasm(s *Suite) []byte {
	s.T.Helper()
	path := filepath.Join(s.Env.RepoRoot, "tests", "upgrade", "testdata", "hackatom.wasm")
	wasm, err := os.ReadFile(path)
	require.NoError(s.T, err, "read hackatom wasm")
	if ioutils.IsWasm(wasm) {
		wasm, err = ioutils.GzipIt(wasm)
		require.NoError(s.T, err, "gzip hackatom wasm")
	} else {
		require.True(s.T, ioutils.IsGzip(wasm), "hackatom wasm should be raw wasm or gzip")
	}
	return wasm
}

func hackatomInstantiateMsg(verifier, beneficiary sdk.AccAddress) wasmtypes.RawContractMessage {
	return wasmtypes.RawContractMessage(fmt.Sprintf(`{"verifier":%q,"beneficiary":%q}`, verifier.String(), beneficiary.String()))
}

func latestWasmCodeID(s *Suite, q wasmtypes.QueryClient) uint64 {
	s.T.Helper()
	var latest uint64
	var next []byte
	for {
		resp, err := q.Codes(s.Ctx, &wasmtypes.QueryCodesRequest{Pagination: &sdkquery.PageRequest{Key: next, Limit: 100}})
		require.NoError(s.T, err, "wasm Codes")
		for _, info := range resp.CodeInfos {
			if info.CodeID > latest {
				latest = info.CodeID
			}
		}
		if resp.Pagination == nil || len(resp.Pagination.NextKey) == 0 {
			break
		}
		next = resp.Pagination.NextKey
	}
	require.NotZero(s.T, latest, "wasm code id should exist after store")
	return latest
}

func latestContractForCode(s *Suite, q wasmtypes.QueryClient, codeID uint64) string {
	s.T.Helper()
	resp, err := q.ContractsByCode(s.Ctx, &wasmtypes.QueryContractsByCodeRequest{
		CodeId:     codeID,
		Pagination: &sdkquery.PageRequest{Limit: 1000},
	})
	require.NoError(s.T, err, "wasm ContractsByCode")
	require.NotEmpty(s.T, resp.Contracts, "stored code should have an instantiated contract")
	return resp.Contracts[len(resp.Contracts)-1]
}
