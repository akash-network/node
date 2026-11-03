package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	abci "github.com/cometbft/cometbft/abci/types"
	cmttypes "github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/baseapp"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/types/kv"
	"github.com/cosmos/cosmos-sdk/types/module"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/simulation"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	epochstypes "pkg.akt.dev/go/node/epochs/v1beta1"
	types "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/app"
	"pkg.akt.dev/node/v2/x/sink"
	sinkkeeper "pkg.akt.dev/node/v2/x/sink/keeper"
	sinksims "pkg.akt.dev/node/v2/x/sink/simulation"
)

func newSinkSimulationApp(t *testing.T) *app.AkashApp {
	t.Helper()
	root := filepath.Join("..", ".cache", "sink-review-corrections", "simulation")
	require.NoError(t, os.MkdirAll(root, 0o700))
	home, err := os.MkdirTemp(root, "app-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(home)) })

	a := app.Setup(app.WithHome(home), app.WithCheckTx(true), app.WithChainID(AppChainID))
	t.Cleanup(func() { require.NoError(t, a.Close()) })

	return a
}

func registeredSinkSimulation(t *testing.T, a *app.AkashApp) sink.AppModule {
	t.Helper()

	for _, mod := range a.SimulationManager().Modules {
		if mod, ok := mod.(sink.AppModule); ok {
			return mod
		}
	}

	t.Fatal("Sink is missing from the application SimulationManager")

	return sink.AppModule{}
}

func sinkSimulationGenesis(t *testing.T, a *app.AkashApp, r *rand.Rand, accounts []simtypes.Account) app.GenesisState {
	t.Helper()
	genesisAccounts := make([]authtypes.GenesisAccount, 0, len(accounts))
	balances := make([]banktypes.Balance, 0, len(accounts))
	validators := make([]*cmttypes.Validator, 0, 64)

	for _, account := range accounts {
		genesisAccounts = append(genesisAccounts, authtypes.NewBaseAccountWithAddress(account.Address))
		balances = append(balances, banktypes.Balance{
			Address: account.Address.String(),
			Coins:   sdk.NewCoins(sdk.NewInt64Coin(sdkutil.DenomUakt, 1_000_000_000)),
		})
	}

	for _, account := range simtypes.RandomAccounts(r, 64) {
		pubkey, err := cryptocodec.ToCmtPubKeyInterface(account.ConsKey.PubKey())
		require.NoError(t, err)
		validators = append(validators, cmttypes.NewValidator(pubkey, 1))
	}

	state, err := simtestutil.GenesisStateWithValSet(a.AppCodec(), app.NewDefaultGenesisState(a.AppCodec()), cmttypes.NewValidatorSet(validators), genesisAccounts, balances...)
	require.NoError(t, err)

	// The SDK helper funds only one validator's bonded pool balance.
	var bank banktypes.GenesisState
	a.AppCodec().MustUnmarshalJSON(state[banktypes.ModuleName], &bank)

	for i := range bank.Balances {
		if bank.Balances[i].Address == authtypes.NewModuleAddress(stakingtypes.BondedPoolName).String() {
			bank.Balances[i].Coins = sdk.NewCoins(sdk.NewCoin(sdkutil.DenomUakt, sdk.DefaultPowerReduction.MulRaw(int64(len(validators)))))
		}
	}

	state[banktypes.ModuleName] = a.AppCodec().MustMarshalJSON(&bank)
	slashing := slashingtypes.DefaultGenesisState()
	for _, validator := range validators {
		address := sdk.ConsAddress(validator.Address)
		slashing.SigningInfos = append(slashing.SigningInfos, slashingtypes.SigningInfo{
			Address:              address.String(),
			ValidatorSigningInfo: slashingtypes.NewValidatorSigningInfo(address, 0, 0, time.Unix(0, 0).UTC(), false, 0),
		})
	}

	state[slashingtypes.ModuleName] = a.AppCodec().MustMarshalJSON(slashing)
	beforeEpochs := bytes.Clone(state[epochstypes.ModuleName])
	mod := registeredSinkSimulation(t, a)
	mod.GenerateGenesisState(&module.SimulationState{
		AppParams: make(simtypes.AppParams),
		Cdc:       a.AppCodec(),
		Rand:      r,
		GenState:  state,
		Accounts:  accounts,
	})

	require.Equal(t, beforeEpochs, []byte(state[epochstypes.ModuleName]), "Sink must not change the native epoch configuration")

	var genesis types.GenesisState
	a.AppCodec().MustUnmarshalJSON(state[types.ModuleName], &genesis)
	require.NoError(t, genesis.Validate())
	require.True(t, genesis.Params.Enabled)
	require.True(t, genesis.Params.RefundsEnabled)
	require.Equal(t, types.DefaultParams().ReporterChallengePeriod, genesis.Params.ReporterChallengePeriod)
	require.NotEmpty(t, genesis.Policies[0].Members)

	return state
}

func sinkSimulationOperations(t *testing.T, a *app.AkashApp) []simtypes.WeightedOperation {
	t.Helper()
	mod := registeredSinkSimulation(t, a)
	operations := mod.WeightedOperations(module.SimulationState{
		AppParams: make(simtypes.AppParams),
		Cdc:       a.AppCodec(),
		TxConfig:  a.TxConfig(),
	})

	require.Len(t, operations, 3)

	for _, operation := range operations {
		require.Positive(t, operation.Weight())
	}

	return operations
}

// Governance executes proposal messages through this same registered message router.
// This focused simulation exercises that dispatch, not proposal voting or Solana finality.
func sinkSimulationVoidProposal(t *testing.T, a *app.AkashApp) simtypes.Operation {
	t.Helper()
	var generate simtypes.MsgSimulatorFn
	for _, proposal := range a.SimulationManager().GetProposalMsgs(module.SimulationState{}) {
		if proposal.AppParamsKey() == sinksims.OpWeightMsgRequestVoid {
			generate = proposal.MsgSimulatorFn()
		}
	}

	require.NotNil(t, generate, "Sink governance proposals are not registered")

	return func(r *rand.Rand, bapp *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account, _ string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		msg := generate(r, ctx, accounts)
		if msg == nil {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(&types.MsgRequestVoid{}), "no transfer available for a void order"), nil, nil
		}

		handler := bapp.MsgServiceRouter().Handler(msg)
		require.NotNil(t, handler)
		cache, write := ctx.CacheContext()
		_, err := handler(cache, msg)
		if err != nil {
			return simtypes.NewOperationMsg(msg, false, "governance dispatch failed"), nil, err
		}

		write()

		return simtypes.NewOperationMsg(msg, true, "governance message dispatch"), nil, nil
	}
}

func TestSinkSimulationLifecycle(t *testing.T) {
	for _, seed := range []int64{42, 73} {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			defer func() { require.False(t, t.Skipped(), "Sink simulation must run to completion") }()
			var previousHash []byte
			var previousGenesis []byte
			var previousCounts map[string]int
			for run := 0; run < 2; run++ {
				a := newSinkSimulationApp(t)
				counts := make(map[string]int)
				noops := make(map[string]int)
				operations := sinkSimulationOperations(t, a)
				operations = append(operations, simulation.NewWeightedOperation(60, sinkSimulationVoidProposal(t, a)))
				for i, weighted := range operations {
					operation := weighted.Op()
					operations[i] = simulation.NewWeightedOperation(weighted.Weight(), func(r *rand.Rand, bapp *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account, chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
						balances := make(map[string]sdkmath.Int, len(accounts))
						for _, account := range accounts {
							balances[account.Address.String()] = a.Keepers.Cosmos.Bank.GetBalance(ctx, account.Address, sdkutil.DenomUakt).Amount
						}

						escrow := a.Keepers.Cosmos.Bank.GetBalance(ctx, sinkkeeper.Address(), sdkutil.DenomUakt).Amount
						msg, future, err := operation(r, bapp, ctx, accounts, chainID)
						require.NoError(t, err)
						assertSinkSimulationBalances(t, a, ctx, msg, balances, escrow)

						if msg.OK {
							counts[msg.Name]++
						} else {
							noops[msg.Comment]++
						}

						require.NoError(t, a.Keepers.Akash.Sink.CheckBalance(ctx))

						return msg, future, err
					})
				}

				r := rand.New(rand.NewSource(seed)) //nolint:gosec
				accounts := simtypes.RandomAccounts(r, 8)
				state := sinkSimulationGenesis(t, a, r, accounts)
				stateBytes, err := json.Marshal(state)
				require.NoError(t, err)
				now := time.Unix(1_750_000_000, 0).UTC()
				_, err = a.InitChain(&abci.RequestInitChain{
					ChainId:       AppChainID,
					Time:          now,
					AppStateBytes: stateBytes,
				})
				require.NoError(t, err)
				var choices []simtypes.Operation
				for _, weighted := range operations {
					for range weighted.Weight() {
						choices = append(choices, weighted.Op())
					}
				}

				// Keep producing operations beyond the challenge deadline. The SDK runner's
				// random empty blocks and validator churn are covered separately below.
				for height := int64(1); height <= 64; height++ {
					_, err = a.FinalizeBlock(&abci.RequestFinalizeBlock{
						Height: height,
						Time:   now,
					})
					require.NoError(t, err)
					ctx := a.GetContextForFinalizeBlock(nil)
					for range 12 {
						_, future, err := choices[r.Intn(len(choices))](r, a.BaseApp, ctx, accounts, AppChainID)
						require.NoError(t, err)
						require.Empty(t, future)
					}

					a.SimWriteState()
					_, err = a.Commit()
					require.NoError(t, err)
					now = now.Add(6 * time.Hour)
				}

				t.Logf("seed=%d run=%d operations=%v noops=%v", seed, run, counts, noops)

				for _, msg := range []sdk.Msg{&types.MsgSink{}, &types.MsgReportOutcome{}, &types.MsgRequestVoid{}, &types.MsgFinalizeRefund{}} {
					require.Positive(t, counts[sdk.MsgTypeURL(msg)], "operation never executed: %T", msg)
				}

				ctx := a.GetContextForCheckTx(nil)
				genesis, err := a.Keepers.Akash.Sink.ExportGenesis(ctx)
				require.NoError(t, err)
				require.NoError(t, genesis.Validate())
				require.Positive(t, genesis.Totals.MintedCount)
				require.Positive(t, genesis.Totals.RefundedCount)
				require.Greater(t, genesis.SendUsage.Epoch, uint64(1))
				assertSinkSimulationIDs(t, genesis.Records)
				assertSinkSimulationDecoder(t, a, ctx)
				assertSinkSimulationImport(t, a, ctx, genesis)

				bz := a.AppCodec().MustMarshalJSON(genesis)
				if run > 0 {
					require.Equal(t, previousHash, a.LastCommitID().Hash, "seed must reproduce committed app state")
					require.Equal(t, previousGenesis, []byte(bz))
					require.Equal(t, previousCounts, counts)
				}

				previousHash = bytes.Clone(a.LastCommitID().Hash)
				previousGenesis = bytes.Clone(bz)
				previousCounts = counts
			}
		})
	}
}

func assertSinkSimulationIDs(t *testing.T, records []types.GenesisRecord) {
	t.Helper()
	byHeight := make(map[int64]map[uint64]bool)
	multiple := false

	for _, record := range records {
		require.NoError(t, record.ID.ValidateBasic())
		sequences := byHeight[record.ID.Height]
		if sequences == nil {
			sequences = make(map[uint64]bool)
			byHeight[record.ID.Height] = sequences
		}

		require.False(t, sequences[record.ID.Sequence], "duplicate per-block sequence")
		sequences[record.ID.Sequence] = true
	}

	for _, sequences := range byHeight {
		multiple = multiple || len(sequences) > 1

		for i := 1; i <= len(sequences); i++ {
			require.True(t, sequences[uint64(i)], "sequences must restart at one each block")
		}
	}

	require.True(t, multiple, "simulation must execute multiple deposits in a block")
}

func assertSinkSimulationDecoder(t *testing.T, a *app.AkashApp, ctx sdk.Context) {
	t.Helper()
	decode := a.SimulationManager().StoreDecoders[types.StoreKey]
	require.NotNil(t, decode, "Sink store decoder is not registered")
	iterator := ctx.KVStore(a.GetKey(types.StoreKey)).Iterator(nil, nil)
	defer func() { require.NoError(t, iterator.Close()) }()
	count := 0
	for ; iterator.Valid(); iterator.Next() {
		pair := kv.Pair{
			Key:   iterator.Key(),
			Value: iterator.Value(),
		}

		require.NotEmpty(t, decode(pair, pair))
		count++
	}

	require.Greater(t, count, 10, "decoder must inspect populated Sink state")
}

func assertSinkSimulationImport(t *testing.T, a *app.AkashApp, ctx sdk.Context, before *types.GenesisState) {
	t.Helper()
	exported, err := a.ExportAppStateAndValidators(false, nil, nil)
	require.NoError(t, err)
	restored := newSinkSimulationApp(t)
	_, err = restored.InitChain(&abci.RequestInitChain{
		ChainId:       AppChainID,
		Time:          ctx.BlockTime(),
		InitialHeight: exported.Height,
		AppStateBytes: exported.AppState,
	})
	require.NoError(t, err)
	restoredContext := restored.NewContext(false)
	after, err := restored.Keepers.Akash.Sink.ExportGenesis(restoredContext)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Equal(t, a.Keepers.Cosmos.Bank.GetAllBalances(ctx, sinkkeeper.Address()), restored.Keepers.Cosmos.Bank.GetAllBalances(restoredContext, sinkkeeper.Address()))
	stateBefore, err := a.Keepers.Akash.Sink.NewQuerier().State(ctx, nil)
	require.NoError(t, err)
	stateAfter, err := restored.Keepers.Akash.Sink.NewQuerier().State(restoredContext, nil)
	require.NoError(t, err)
	require.Equal(t, stateBefore, stateAfter)
	require.NoError(t, restored.Keepers.Akash.Sink.CheckBalance(restoredContext))
	assertSinkSimulationDecoder(t, restored, restoredContext)
}

func TestSinkSimulationQuotaAndAnte(t *testing.T) {
	a := newSinkSimulationApp(t)
	r := rand.New(rand.NewSource(19)) //nolint:gosec
	accounts := simtypes.RandomAccounts(r, 8)
	state := sinkSimulationGenesis(t, a, r, accounts)
	bz, err := json.Marshal(state)
	require.NoError(t, err)
	now := time.Unix(1_750_000_000, 0).UTC()
	_, err = a.InitChain(&abci.RequestInitChain{
		ChainId:       AppChainID,
		Time:          now,
		AppStateBytes: bz,
	})
	require.NoError(t, err)
	_, err = a.FinalizeBlock(&abci.RequestFinalizeBlock{
		Height: 1,
		Time:   now,
	})
	require.NoError(t, err)
	ctx := a.GetContextForFinalizeBlock(nil)
	operations := sinkSimulationOperations(t, a)
	deposit := operations[0].Op()
	operation, _, err := deposit(r, a.BaseApp, ctx, accounts, AppChainID)
	require.NoError(t, err)
	require.True(t, operation.OK, operation.Comment)
	var msg types.MsgSink
	require.NoError(t, a.AppCodec().Unmarshal(operation.Msg, &msg))
	before, err := a.Keepers.Akash.Sink.ExportGenesis(ctx)
	require.NoError(t, err)

	owner, err := sdk.AccAddressFromBech32(msg.Sender)
	require.NoError(t, err)
	account, found := simtypes.FindAccount(accounts, owner)
	require.True(t, found)
	acc := a.Keepers.Cosmos.Acct.GetAccount(ctx, owner)
	tx, err := simtestutil.GenSignedMockTx(r, a.TxConfig(), []sdk.Msg{&msg, &msg}, nil, simtestutil.DefaultGenTxGas, AppChainID, []uint64{acc.GetAccountNumber()}, []uint64{acc.GetSequence()}, account.PrivKey)
	require.NoError(t, err)
	_, _, err = a.SimDeliver(a.TxConfig().TxEncoder(), tx)
	require.ErrorIs(t, err, sdkerrors.ErrInvalidRequest)
	require.ErrorContains(t, err, "sink deposit must be the only transaction message")
	after, err := a.Keepers.Akash.Sink.ExportGenesis(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after, "batched deposits must not alter Sink state")

	completed := 1
	for ; completed < 300; completed++ {
		operation, _, err = deposit(r, a.BaseApp, ctx, accounts, AppChainID)
		require.NoError(t, err)

		if !operation.OK {
			break
		}
	}

	require.Less(t, completed, 300, "simulation must encounter its configured send quota")
	require.Equal(t, "no funded account with send quota", operation.Comment)
	quota, err := a.Keepers.Akash.Sink.NewQuerier().State(ctx, nil)
	require.NoError(t, err)
	params, err := a.Keepers.Akash.Sink.GetParams(ctx)
	require.NoError(t, err)
	require.True(t, params.MaxTransferAmountPerEpoch.Amount.Sub(quota.SendUsage.Amount.Amount).LT(params.MinAmountPerTransfer.Amount))

	// A real rejected transaction must leave the same quota and transfer state.
	before, err = a.Keepers.Akash.Sink.ExportGenesis(ctx)
	require.NoError(t, err)
	msg.Amount = params.MaxAmountPerTransfer
	acc = a.Keepers.Cosmos.Acct.GetAccount(ctx, owner)
	tx, err = simtestutil.GenSignedMockTx(r, a.TxConfig(), []sdk.Msg{&msg}, nil, simtestutil.DefaultGenTxGas, AppChainID, []uint64{acc.GetAccountNumber()}, []uint64{acc.GetSequence()}, account.PrivKey)
	require.NoError(t, err)
	_, _, err = a.SimDeliver(a.TxConfig().TxEncoder(), tx)
	require.Error(t, err)
	require.True(t, types.ErrSendCapReached.Is(err) || types.ErrAccountDailyAmountExceeded.Is(err), "%v", err)
	after, err = a.Keepers.Akash.Sink.ExportGenesis(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.NoError(t, a.Keepers.Akash.Sink.CheckBalance(ctx))

	epoch, err := a.Keepers.Akash.Epochs.GetEpoch(ctx, "day")
	require.NoError(t, err)
	a.SimWriteState()
	_, err = a.Commit()
	require.NoError(t, err)
	_, err = a.FinalizeBlock(&abci.RequestFinalizeBlock{
		Height: 2,
		Time:   epoch.CurrentEpochStartTime.Add(epoch.Duration + time.Nanosecond),
	})
	require.NoError(t, err)
	ctx = a.GetContextForFinalizeBlock(nil)
	rolled, err := a.Keepers.Akash.Sink.NewQuerier().State(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, quota.Epoch+1, rolled.Epoch)
	require.True(t, rolled.SendUsage.Amount.IsZero())
	require.Equal(t, quota.Totals, rolled.Totals)
	operation, _, err = deposit(r, a.BaseApp, ctx, accounts, AppChainID)
	require.NoError(t, err)
	require.True(t, operation.OK, operation.Comment)
	after, err = a.Keepers.Akash.Sink.ExportGenesis(ctx)
	require.NoError(t, err)
	assertSinkSimulationIDs(t, after.Records)
	t.Logf("seed=19 deposits before quota=%d, rejected batched/quota transactions=2, deposits after native epoch rollover=1", completed)

	var proposal *types.MsgUpdateParams
	for _, weighted := range a.SimulationManager().GetProposalMsgs(module.SimulationState{}) {
		if weighted.AppParamsKey() == sinksims.OpWeightMsgUpdateParams {
			proposal = weighted.MsgSimulatorFn()(r, ctx, accounts).(*types.MsgUpdateParams)
		}
	}

	require.NotNil(t, proposal, "Sink parameter proposals must be registered")
	require.NoError(t, proposal.ValidateBasic())
	handler := a.MsgServiceRouter().Handler(proposal)
	require.NotNil(t, handler)
	unauthorized := *proposal
	unauthorized.Authority = account.Address.String()
	_, err = handler(ctx, &unauthorized)
	require.ErrorIs(t, err, sdkerrors.ErrUnauthorized)
	_, err = handler(ctx, proposal)
	require.NoError(t, err)
	params, err = a.Keepers.Akash.Sink.GetParams(ctx)
	require.NoError(t, err)
	require.Equal(t, proposal.Params, params)
	require.Equal(t, types.DefaultParams().ReporterChallengePeriod, params.ReporterChallengePeriod)
}

func TestSinkSimulationFromSeed(t *testing.T) {
	for _, seed := range []int64{42, 73} {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			defer func() { require.False(t, t.Skipped(), "SDK simulation must execute Sink operations") }()
			a := newSinkSimulationApp(t)
			counts := make(map[string]int)
			noops := make(map[string]int)
			operations := sinkSimulationOperations(t, a)
			for i, weighted := range operations {
				operation := weighted.Op()
				operations[i] = simulation.NewWeightedOperation(weighted.Weight(), func(r *rand.Rand, bapp *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account, chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
					msg, future, err := operation(r, bapp, ctx, accounts, chainID)
					if msg.OK {
						counts[msg.Name]++
					} else {
						noops[msg.Comment]++
					}

					return msg, future, err
				})
			}

			config := simtypes.Config{
				Seed:               seed,
				InitialBlockHeight: 1,
				NumBlocks:          8,
				BlockSize:          20,
				ChainID:            AppChainID,
				Commit:             true,
				Lean:               true,
			}
			genesisFn := func(r *rand.Rand, accounts []simtypes.Account, config simtypes.Config) (json.RawMessage, []simtypes.Account, string, time.Time) {
				state := sinkSimulationGenesis(t, a, r, accounts)
				bz, err := json.Marshal(state)
				require.NoError(t, err)

				return bz, accounts, config.ChainID, time.Unix(1_750_000_000, 0).UTC()
			}
			_, _, err := simulation.SimulateFromSeedX(t, log.NewNopLogger(), io.Discard, a.BaseApp, genesisFn, func(r *rand.Rand, _ int) []simtypes.Account {
				return simtypes.RandomAccounts(r, 8)
			}, operations, a.ModuleAccountAddrs(), config, a.AppCodec(), simulation.NewLogWriter(true))
			require.NoError(t, err)
			t.Logf("SDK seed=%d operations=%v noops=%v", seed, counts, noops)
			require.Positive(t, counts[sdk.MsgTypeURL(&types.MsgSink{})])
			require.Positive(t, counts[sdk.MsgTypeURL(&types.MsgReportOutcome{})])
			require.Equal(t, int64(8), a.LastBlockHeight())
			require.NoError(t, a.Keepers.Akash.Sink.CheckBalance(a.GetContextForCheckTx(nil)))
		})
	}
}

func assertSinkSimulationBalances(t *testing.T, a *app.AkashApp, ctx sdk.Context, operation simtypes.OperationMsg, balances map[string]sdkmath.Int, escrow sdkmath.Int) {
	t.Helper()

	if operation.OK {
		switch operation.Name {
		case sdk.MsgTypeURL(&types.MsgSink{}):
			var msg types.MsgSink
			require.NoError(t, a.AppCodec().Unmarshal(operation.Msg, &msg))
			balances[msg.Sender] = balances[msg.Sender].Sub(msg.Amount.Amount).SubRaw(1)
			escrow = escrow.Add(msg.Amount.Amount)
		case sdk.MsgTypeURL(&types.MsgReportOutcome{}):
			var msg types.MsgReportOutcome
			require.NoError(t, a.AppCodec().Unmarshal(operation.Msg, &msg))
			balances[msg.Reporter] = balances[msg.Reporter].SubRaw(1)
			transfer, err := a.Keepers.Akash.Sink.GetTransfer(ctx, msg.ID)
			require.NoError(t, err)
			found := false
			for _, vote := range transfer.Settlement.Votes {
				found = found || vote.Reporter == msg.Reporter && vote.Observation.Outcome == msg.Observation.Outcome
			}

			require.True(t, found, "successful report must retain the reporter's vote")
		case sdk.MsgTypeURL(&types.MsgFinalizeRefund{}):
			var msg types.MsgFinalizeRefund
			require.NoError(t, a.AppCodec().Unmarshal(operation.Msg, &msg))
			transfer, err := a.Keepers.Akash.Sink.GetTransfer(ctx, msg.ID)
			require.NoError(t, err)
			require.Equal(t, types.StatusRefunded, transfer.Status)
			balances[msg.Executor] = balances[msg.Executor].SubRaw(1)
			balances[msg.ID.Owner] = balances[msg.ID.Owner].Add(transfer.Data.Amount.Amount)
			escrow = escrow.Sub(transfer.Data.Amount.Amount)
		case sdk.MsgTypeURL(&types.MsgRequestVoid{}):
			var msg types.MsgRequestVoid
			require.NoError(t, a.AppCodec().Unmarshal(operation.Msg, &msg))
			transfer, err := a.Keepers.Akash.Sink.GetTransfer(ctx, msg.ID)
			require.NoError(t, err)
			require.Equal(t, types.StatusVoided, transfer.Status)
			require.NotNil(t, transfer.Settlement.VoidAuthorization)
		default:
			t.Fatalf("unexpected simulated message %s", operation.Name)
		}
	}

	for owner, amount := range balances {
		address, err := sdk.AccAddressFromBech32(owner)
		require.NoError(t, err)
		require.Equal(t, amount, a.Keepers.Cosmos.Bank.GetBalance(ctx, address, sdkutil.DenomUakt).Amount)
	}

	require.Equal(t, escrow, a.Keepers.Cosmos.Bank.GetBalance(ctx, sinkkeeper.Address(), sdkutil.DenomUakt).Amount)
}
