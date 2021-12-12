package cli

import (
	"bufio"

	cinpuit "github.com/cosmos/cosmos-sdk/client/input"
	"github.com/spf13/cobra"
)

func GetConfirmation(cmd *cobra.Command, prompt string) (bool, error) {
	return cinpuit.GetConfirmation(prompt, bufio.NewReader(cmd.InOrStdin()), cmd.ErrOrStderr())
}
