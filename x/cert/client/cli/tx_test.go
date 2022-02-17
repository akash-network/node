package cli

import (
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"testing"
)

func TestGenerateServer(t *testing.T){
	sdkclient.GetClientTxContext(cmd)

	args := []string{
		host,
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)
	return clitestutil.ExecTestCLICmd(clientCtx, cmdGenerateServer(), args)

}