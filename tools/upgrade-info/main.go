package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"

	utilcli "pkg.akt.dev/node/util/cli"
)

func main() {
	cmd := cobra.Command{
		Use:     "upgrade-info",
		Example: "upgrade-info <tag> <file>",
		Args:    cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			info, err := utilcli.UpgradeInfoFromTag(cmd.Context(), args[0], true)
			if err != nil {
				return err
			}

			if len(args) == 1 {
				fmt.Printf("%s\n", info)
				return nil
			}

			file, err := os.Create(args[1])
			if err != nil {
				return err
			}
			defer func() {
				_ = file.Close()
			}()

			if _, err = file.WriteString(info); err != nil {
				return err
			}
			return nil
		},
	}
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		log.Fatal(err)
	}
}
