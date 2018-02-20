package main

import (
	"encoding/json"
	"io/ioutil"
	"os"

	ptypes "github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/types"
)

const (
	maxTokens uint64 = 1000000000
	chainID          = "local"
)

func initCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init [address]",
		Short: "Initialize node",
		Args:  cobra.ExactArgs(1),
		RunE:  withContext(doInitCommand),
	}
	return cmd
}

func doInitCommand(ctx Context, cmd *cobra.Command, args []string) error {

	cfg, err := ctx.TMConfig()
	if err != nil {
		return err
	}

	pval := types.GenPrivValidatorFS("")

	// photon genesis

	addr := new(base.Bytes)
	if err := addr.DecodeString(args[0]); err != nil {
		return err
	}

	pgenesis := &ptypes.Genesis{
		Accounts: []ptypes.Account{
			ptypes.Account{Address: *addr, Balance: maxTokens},
		},
	}

	doc := types.GenesisDoc{
		ChainID: chainID,
		Validators: []types.GenesisValidator{
			types.GenesisValidator{
				Name:   "root",
				Power:  10,
				PubKey: pval.PubKey,
			},
		},
		AppOptions: pgenesis,
	}

	if err := doc.ValidateAndComplete(); err != nil {
		return err
	}
	if err := writeObj(doc, cfg.GenesisFile(), 0644); err != nil {
		return err
	}
	if err := writeObj(pval, cfg.PrivValidatorFile(), 0400); err != nil {
		return err
	}
	return nil
}

func writeObj(obj interface{}, path string, perm os.FileMode) error {
	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}
	_, err = os.Stat(path)
	if !os.IsNotExist(err) {
		return nil
	}
	err = ioutil.WriteFile(path, data, perm)
	if err != nil {
		return err
	}
	return nil
}
