package testnetify

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/crypto/tmhash"

	"github.com/tendermint/tendermint/node"
	pvm "github.com/tendermint/tendermint/privval"
	cmtstate "github.com/tendermint/tendermint/proto/tendermint/state"
	cmtproto "github.com/tendermint/tendermint/proto/tendermint/types"
	"github.com/tendermint/tendermint/proxy"
	"github.com/tendermint/tendermint/store"
	cmttypes "github.com/tendermint/tendermint/types"
	dbm "github.com/tendermint/tm-db"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdksrv "github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/types"

	akash "github.com/akash-network/node/app"
)

const (
	// testnet keys

	FlagTraceStore     = "trace-store"
	FlagTestnetRootDir = "testnet-rootdir"
	KeyTestnetRootDir  = FlagTestnetRootDir

	KeyIsTestnet             = "is-testnet"
	KeyTestnetConfig         = "testnet-config"
	KeyTestnetTriggerUpgrade = "testnet-trigger-upgrade"
)

// GetCmd uses the provided chainID and operatorAddress as well as the local private validator key to
// control the network represented in the data folder. This is useful to create testnets nearly identical to your
// mainnet environment.
func GetCmd(testnetAppCreator types.AppCreator) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "testnetify",
		Short: "Create a testnet from current local state",
		Long: `Create a testnet from current local state.
After utilizing this command the network will start. If the network is stopped,
the normal "start" command should be used. Re-using this command on state that
has already been modified by this command could result in unexpected behavior.

Additionally, the first block may take a few minutes to be committed, depending
on how old the block is. For instance, if a snapshot was taken weeks ago and we want
to turn this into a testnet, it is possible lots of pending state needs to be committed
(expiring locks, etc.). It is recommended that you should wait for this block to be committed
before stopping the daemon.

If the --trigger-testnet-upgrade flag is set, the upgrade handler specified by the flag will be run
on the first block of the testnet.

Regardless of whether the flag is set or not, if any new stores are introduced in the daemon being run,
those stores will be registered in order to prevent panics. Therefore, you only need to set the flag if
you want to test the upgrade handler itself.
`,
		Example: "testnetify",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sctx := sdksrv.GetServerContextFromCmd(cmd)
			cctx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			_, err = sdksrv.GetPruningOptionsFromFlags(sctx.Viper)
			if err != nil {
				return err
			}

			rootDir, err := cmd.Flags().GetString(KeyTestnetRootDir)
			if err != nil {
				return err
			}

			sctx.Logger.Info("testnetifying blockchain state")
			cfg := TestnetConfig{}

			cfgFilePath, err := cmd.Flags().GetString(KeyTestnetConfig)
			if err != nil {
				return err
			}
			cfgFile, err := os.Open(cfgFilePath)
			if err != nil {
				return err
			}

			defer func() {
				_ = cfgFile.Close()
			}()

			cfgData, err := io.ReadAll(cfgFile)
			if err != nil {
				return err
			}

			if err = json.Unmarshal(cfgData, &cfg); err != nil {
				return err
			}

			for i, acc := range cfg.Accounts {
				if len(acc.Balances) > 0 {
					cfg.Accounts[i].Balances = sdk.NewCoins(acc.Balances...)
				} else {
					cfg.Accounts[i].Balances = sdk.NewCoins(
						sdk.NewInt64Coin("uakt", 1000000000000),
						sdk.NewInt64Coin("ibc/12C6A0C374171B595A0A9E18B83FA09D295FB1F2D8C6DAA3AC28683471752D84", 1000000000000), // axlUSDC
					)
				}
			}

			for i, val := range cfg.Validators {
				if val.Tokens == nil {
					tk := sdk.NewInt(900000000000000)
					cfg.Validators[i].Tokens = &tk
				}

				if val.DelegatorShares == nil {
					shares := sdk.MustNewDecFromStr("10000000")
					cfg.Validators[i].DelegatorShares = &shares
				}
			}

			sctx.Logger.Info(fmt.Sprintf("loaded config from %s", cfgFilePath))

			if name, _ := cmd.Flags().GetString(KeyTestnetTriggerUpgrade); name != "" {
				cfg.upgrade.Name = name
			}

			if skip, _ := cmd.Flags().GetBool(flags.FlagSkipConfirmation); !skip {
				// Confirmation prompt to prevent accidental modification of state.
				reader := bufio.NewReader(os.Stdin)
				fmt.Println("This operation will modify state in your data folder and cannot be undone. Do you want to continue? (y/n)")
				text, _ := reader.ReadString('\n')
				response := strings.TrimSpace(strings.ToLower(text))
				if response != "y" && response != "yes" {
					fmt.Println("Operation canceled.")
					return nil
				}
			}

			for i := range cfg.Validators {
				cfg.Validators[i].Home = filepath.Join(rootDir, cfg.Validators[i].Home)
			}

			home := sctx.Config.RootDir
			db, err := openDB(home)
			if err != nil {
				return err
			}

			traceWriter, traceCleanupFn, err := setupTraceWriter(sctx)
			if err != nil {
				return err
			}

			app, err := testnetify(sctx, cfg, testnetAppCreator, db, traceWriter)
			if err != nil {
				return err
			}

			defer func() {
				traceCleanupFn()

				if localErr := db.Close(); localErr != nil {
					sctx.Logger.Error(localErr.Error())
				}
			}()

			go func() {
				defer func() {
					if proc, err := os.FindProcess(os.Getpid()); err == nil {
						_ = proc.Signal(os.Interrupt)
					}
				}()
				ctx := cmd.Context()

				cctx, err := client.GetClientQueryContext(cmd)
				if err != nil {
					return
				}

				ticker := time.NewTicker(time.Second)
				timeout := time.After(1 * time.Minute)

				var h int64

			loop:
				for {
					select {
					case <-timeout:
						ticker.Stop()
						return
					case <-ticker.C:
						status, err := cctx.Client.Status(ctx)
						if err == nil {
							h = status.SyncInfo.LatestBlockHeight
							break loop
						}
					}
				}

				ticker = time.NewTicker(time.Second)
				timeout = time.After(1 * time.Minute)

				for {
					select {
					case <-timeout:
						ticker.Stop()
						return
					case <-ticker.C:
						status, err := cctx.Client.Status(ctx)
						if err == nil && status != nil {
							if status.SyncInfo.LatestBlockHeight > h+1 {
								return
							}
						}
					}
				}
			}()

			err = sdksrv.StartInProcess(sctx, cctx, app)
			if err != nil && !strings.Contains(err.Error(), "130") {
				sctx.Logger.Error("testnetify finished with error", "err", err.Error())
				return err
			}

			sctx.Logger.Info("testnetify completed")

			return nil
		},
	}

	cmd.Flags().Bool(flags.FlagSkipConfirmation, false, "Skip the confirmation prompt")
	cmd.Flags().String(KeyTestnetTriggerUpgrade, "", "If set (example: \"v1.0.0\"), triggers the v1.0.0 upgrade handler to run on the first block of the testnet")
	cmd.Flags().StringP(KeyTestnetConfig, "c", "", "testnet config file config file")
	cmd.Flags().String(KeyTestnetRootDir, "", "path to where testnet validators are located")
	cmd.Flags().String(flags.FlagNode, "tcp://localhost:26657", "")
	_ = cmd.MarkFlagRequired(KeyTestnetConfig)
	_ = cmd.MarkFlagRequired(KeyTestnetRootDir)

	cmd.MarkFlagsRequiredTogether(KeyTestnetConfig, KeyTestnetRootDir)

	return cmd
}

// testnetify modifies both state and blockStore, allowing the provided operator address and local validator key to control the network
// that the state in the data folder represents. The chainID of the local genesis file is modified to match the provided chainID.
func testnetify(sctx *sdksrv.Context, tcfg TestnetConfig, testnetAppCreator types.AppCreator, db dbm.DB, traceWriter io.WriteCloser) (types.Application, error) {
	cfg := sctx.Config

	thisVal := cfg.PrivValidatorKeyFile()
	sort.Slice(tcfg.Validators, func(i, j int) bool {
		return thisVal == tcfg.Validators[i].Home
	})

	newChainID := tcfg.ChainID

	// Modify app genesis chain ID and save to the genesis file.
	genDocProvider := node.DefaultGenesisDocProviderFunc(cfg)
	cGen, err := genDocProvider()
	if err != nil {
		return nil, err
	}

	cGen.GenesisDoc.ChainID = newChainID
	err = cGen.GenesisDoc.ValidateAndComplete()
	if err != nil {
		return nil, err
	}

	err = cGen.GenesisDoc.SaveAs(cfg.GenesisFile())
	if err != nil {
		return nil, err
	}

	blockStoreDB, err := node.DefaultDBProvider(&node.DBContext{ID: "blockstore", Config: cfg})
	if err != nil {
		return nil, err
	}
	blockStore := store.NewBlockStore(blockStoreDB)
	defer func() {
		_ = blockStore.Close()
	}()

	stateDB, err := node.DefaultDBProvider(&node.DBContext{ID: "state", Config: cfg})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = stateDB.Close()
	}()

	jsonBlob, err := os.ReadFile(cfg.GenesisFile())
	if err != nil {
		return nil, fmt.Errorf("couldn't read GenesisDoc file: %w", err)
	}

	// Since we modified the chainID, we set the new genesisDocHash in the stateDB.
	updatedChecksum := tmhash.Sum(jsonBlob)

	if err = stateDB.SetSync(node.GenesisDocHashKey, updatedChecksum); err != nil {
		return nil, node.ErrSaveGenesisDocHash{Err: err}
	}

	state, stateStore, _, err := node.LoadStateFromDBOrGenesisDocProvider(stateDB, genDocProvider, hex.EncodeToString(updatedChecksum))
	if err != nil {
		return nil, err
	}

	appConfig := &akash.TestnetConfig{
		Accounts:   tcfg.Accounts,
		Gov:        tcfg.Gov,
		Validators: make([]akash.TestnetValidator, 0, len(tcfg.Validators)),
		Upgrade:    tcfg.upgrade,
	}

	for i, val := range tcfg.Validators {
		configDir := filepath.Join(val.Home, "config")
		dataDir := filepath.Join(val.Home, "data")

		// Regenerate addrbook.json to prevent peers on old network from causing error logs.
		addrBookPath := filepath.Join(configDir, "addrbook.json")
		if err := os.Remove(addrBookPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove existing addrbook.json: %w", err)
		}

		emptyAddrBook := []byte("{}")
		if err := os.WriteFile(addrBookPath, emptyAddrBook, 0o600); err != nil {
			return nil, fmt.Errorf("failed to create empty addrbook.json: %w", err)
		}

		keyFile := filepath.Join(configDir, "priv_validator_key.json")
		stateFile := filepath.Join(dataDir, "priv_validator_state.json")

		privValidator := pvm.LoadOrGenFilePV(keyFile, stateFile)
		pubKey, err := privValidator.GetPubKey()
		if err != nil {
			return nil, err
		}
		validatorAddress := pubKey.Address()

		pubkey := &ed25519.PubKey{Key: pubKey.Bytes()}
		consensusPubkey, err := codectypes.NewAnyWithValue(pubkey)
		if err != nil {
			return nil, err
		}

		appConfig.Validators = append(appConfig.Validators, akash.TestnetValidator{
			OperatorAddress:   val.Operator.Bytes(),
			ConsensusAddress:  pubKey.Address().Bytes(),
			ConsensusPubKey:   consensusPubkey,
			Moniker:           val.Moniker,
			Commission:        val.Commission,
			MinSelfDelegation: val.MinSelfDelegation,
			Tokens:            *val.Tokens,
			DelegatorShares:   *val.DelegatorShares,
			Status:            val.Status,
		})

		tcfg.Validators[i].privValidator = privValidator
		tcfg.Validators[i].pubKey = pubKey
		tcfg.Validators[i].validatorAddress = validatorAddress
		tcfg.Validators[i].consAddress = pubKey.Address().Bytes()
	}

	sctx.Viper.Set(KeyTestnetConfig, appConfig)

	testnetApp := testnetAppCreator(sctx.Logger, db, traceWriter, sctx.Viper)

	// We need to create a temporary proxyApp to get the initial state of the application.
	// Depending on how the node was stopped, the application height can differ from the blockStore height.
	// This height difference changes how we go about modifying the state.
	cmtApp := testnetApp

	clientCreator := proxy.NewLocalClientCreator(cmtApp)

	proxyApp := proxy.NewAppConns(clientCreator)
	if err := proxyApp.Start(); err != nil {
		return nil, fmt.Errorf("error starting proxy app connections: %w", err)
	}
	res, err := proxyApp.Query().InfoSync(proxy.RequestInfo)
	if err != nil {
		return nil, fmt.Errorf("error calling Info: %w", err)
	}
	err = proxyApp.Stop()
	if err != nil {
		return nil, err
	}
	appHash := res.LastBlockAppHash
	appHeight := res.LastBlockHeight

	var block *cmttypes.Block
	switch {
	case appHeight == blockStore.Height():
		block = blockStore.LoadBlock(blockStore.Height())
		// If the state's last blockstore height does not match the app and blockstore height, we likely stopped with the halt height flag.
		if state.LastBlockHeight != appHeight {
			state.LastBlockHeight = appHeight
			block.AppHash = appHash
			state.AppHash = appHash
		} else {
			// Node was likely stopped via SIGTERM, delete the next block's seen commit
			err := blockStoreDB.Delete(fmt.Appendf(nil, "SC:%v", blockStore.Height()+1))
			if err != nil {
				return nil, err
			}
		}
	case blockStore.Height() > state.LastBlockHeight:
		// This state usually occurs when we gracefully stop the node.
		err = blockStore.DeleteLatestBlock()
		if err != nil {
			return nil, err
		}
		block = blockStore.LoadBlock(blockStore.Height())
	default:
		// If there is any other state, we just load the block
		block = blockStore.LoadBlock(blockStore.Height())
	}

	block.ChainID = newChainID
	state.ChainID = newChainID

	block.LastBlockID = state.LastBlockID
	block.LastCommit.BlockID = state.LastBlockID

	newValidators := make([]*cmttypes.Validator, 0, len(tcfg.Validators))

	signatures := make([]cmttypes.CommitSig, 0, len(tcfg.Validators))

	for _, val := range tcfg.Validators {
		// Create a vote from our validator
		vote := cmttypes.Vote{
			Type:             cmtproto.PrecommitType,
			Height:           state.LastBlockHeight,
			Round:            0,
			BlockID:          state.LastBlockID,
			Timestamp:        time.Now(),
			ValidatorAddress: val.validatorAddress,
			ValidatorIndex:   0,
			Signature:        []byte{},
		}

		voteProto := vote.ToProto()

		err = val.privValidator.SignVote(newChainID, voteProto)
		if err != nil {
			return nil, err
		}
		vote.Signature = voteProto.Signature
		vote.Timestamp = voteProto.Timestamp

		signatures = append(signatures, cmttypes.CommitSig{
			BlockIDFlag:      block.LastCommit.Signatures[0].BlockIDFlag,
			ValidatorAddress: val.validatorAddress,
			Timestamp:        voteProto.Timestamp,
			Signature:        voteProto.Signature,
		})

		newValidators = append(newValidators, &cmttypes.Validator{
			Address:     val.validatorAddress,
			PubKey:      val.pubKey,
			VotingPower: val.Tokens.Int64(),
		})
	}

	// Replace all valSets in state to be the valSet with just our validator.
	// and set the very first validator as a proposer
	newValSet := &cmttypes.ValidatorSet{
		Validators: newValidators,
		Proposer:   newValidators[0],
	}

	// Modify the block's lastCommit to be signed only by our validator set
	block.LastCommit.Signatures = signatures

	// Load the seenCommit of the lastBlockHeight and modify it to be signed from our validator
	seenCommit := blockStore.LoadSeenCommit(state.LastBlockHeight)

	seenCommit.BlockID = state.LastBlockID
	seenCommit.Round = 0
	seenCommit.Signatures = signatures

	err = blockStore.SaveSeenCommit(state.LastBlockHeight, seenCommit)
	if err != nil {
		return nil, err
	}

	state.Validators = newValSet
	state.LastValidators = newValSet
	state.NextValidators = newValSet
	state.LastHeightValidatorsChanged = blockStore.Height()

	err = stateStore.Save(state)
	if err != nil {
		return nil, err
	}

	// Create a ValidatorsInfo struct to store in stateDB.
	valSet, err := state.Validators.ToProto()
	if err != nil {
		return nil, err
	}
	valInfo := &cmtstate.ValidatorsInfo{
		ValidatorSet:      valSet,
		LastHeightChanged: state.LastBlockHeight,
	}
	buf, err := valInfo.Marshal()
	if err != nil {
		return nil, err
	}

	// Modify Validators stateDB entry.
	err = stateDB.Set(fmt.Appendf(nil, "validatorsKey:%v", blockStore.Height()), buf)
	if err != nil {
		return nil, err
	}

	// Modify LastValidators stateDB entry.
	err = stateDB.Set(fmt.Appendf(nil, "validatorsKey:%v", blockStore.Height()-1), buf)
	if err != nil {
		return nil, err
	}

	// Modify NextValidators stateDB entry.
	err = stateDB.Set(fmt.Appendf(nil, "validatorsKey:%v", blockStore.Height()+1), buf)
	if err != nil {
		return nil, err
	}

	return testnetApp, err
}
