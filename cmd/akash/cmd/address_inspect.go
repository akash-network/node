package cmd

import (
	"fmt"
	"os"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
)

func cmdAddressInspect() *cobra.Command {
	return &cobra.Command{
		Use: "address-inspect [address]",
		Run: func(_ *cobra.Command, args []string) {
			for _, val := range args {
				address, err := sdk.AccAddressFromBech32(val)
				if err == nil {
					fmt.Printf("%s\t %v\n", address, address)
					continue
				}
				address, err = sdk.AccAddressFromHex(val)
				if err == nil {
					fmt.Printf("%s\t %v\n", address, address)
					continue
				}

				fmt.Fprintf(os.Stderr, "[error] %v: %v\n", address, err)
			}
		},
	}
}
