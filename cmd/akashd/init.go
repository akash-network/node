package main

import (
	ptypes "github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/ovrclk/akash/util/initgen"
	"github.com/spf13/cobra"
)

const (
	maxTokens uint64 = 1000000000

	flagInitCount  = "count"
	flagInitType   = "type"
	flagInitOutput = "out"
	flagInitName   = "name"
)

func initCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "init [address]",
		Short: "Initialize node",
		Args:  cobra.ExactArgs(1),
		RunE:  withContext(doInitCommand),
	}

	cmd.Flags().UintP(flagInitCount, "c", 1, "generate multiple init directories")
	cmd.Flags().StringP(flagInitType, "t", string(initgen.TypeDirectory), "output type (dir,helm)")
	cmd.Flags().StringP(flagInitOutput, "o", "", "output directory (default to -d value)")
	cmd.Flags().StringP(flagInitName, "n", "node", "node name")

	return cmd
}

func doInitCommand(ctx Context, cmd *cobra.Command, args []string) error {

	b := initgen.NewBuilder()

	name, err := cmd.Flags().GetString(flagInitName)
	if err != nil {
		return err
	}
	b = b.WithName(name)

	path, err := cmd.Flags().GetString(flagInitOutput)
	if err != nil {
		return err
	}
	if path == "" {
		path = ctx.RootDir()
	}
	b = b.WithPath(path)

	count, err := cmd.Flags().GetUint(flagInitCount)
	if err != nil {
		return err
	}
	b = b.WithCount(count)

	type_, err := cmd.Flags().GetString(flagInitType)
	if err != nil {
		return err
	}

	pg, err := generateAkashGenesis(cmd, args)
	if err != nil {
		return err
	}
	b = b.WithAkashGenesis(pg)

	wctx, err := b.Create()
	if err != nil {
		return err
	}

	w, err := initgen.CreateWriter(initgen.Type(type_), wctx)
	if err != nil {
		return err
	}

	return w.Write()
}

func generateAkashGenesis(cmd *cobra.Command, args []string) (*ptypes.Genesis, error) {
	addr := new(base.Bytes)
	if err := addr.DecodeString(args[0]); err != nil {
		return nil, err
	}
	return &ptypes.Genesis{
		Accounts: []ptypes.Account{
			ptypes.Account{Address: *addr, Balance: maxTokens},
		},
	}, nil
}
