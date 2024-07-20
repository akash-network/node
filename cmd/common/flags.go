package common

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	FlagDeposit = "deposit"
)

var (
	ErrUnknownSubspace = errors.New("unknown subspace")
)

type paramCoin struct {
	Denom  string
	Amount string
}

type paramCoins []paramCoin

func AddDepositFlags(flags *pflag.FlagSet) {
	flags.String(FlagDeposit, "", "Deposit amount")
}

// func DetectDeposit(ctx context.Context, flags *pflag.FlagSet, cl client.QueryClient, subspace, paramKey string) (sdk.Coin, error) {
// 	var deposit sdk.Coin
// 	var depositStr string
// 	var err error
//
// 	if !flags.Changed(FlagDeposit) {
// 		res, err := cl.Params().Params(ctx, &proposal.QueryParamsRequest{
// 			Subspace: subspace,
// 			Key:      paramKey,
// 		})
// 		if err != nil {
// 			return sdk.Coin{}, err
// 		}
//
// 		switch subspace {
// 		case "market":
// 			var coin paramCoin
//
// 			if err = json.Unmarshal([]byte(res.Param.Value), &coin); err != nil {
// 				return sdk.Coin{}, err
// 			}
//
// 			depositStr = fmt.Sprintf("%s%s", coin.Amount, coin.Denom)
// 		case "deployment":
// 			var coins paramCoins
//
// 			if err = json.Unmarshal([]byte(res.Param.Value), &coins); err != nil {
// 				return sdk.Coin{}, err
// 			}
//
// 			// always default to AKT
// 			for _, sCoin := range coins {
// 				if sCoin.Denom == "uakt" {
// 					depositStr = fmt.Sprintf("%s%s", sCoin.Amount, sCoin.Denom)
// 					break
// 				}
// 			}
// 		default:
// 			return sdk.Coin{}, ErrUnknownSubspace
// 		}
//
// 		if depositStr == "" {
// 			return sdk.Coin{}, fmt.Errorf("couldn't query default deposit amount for uAKT")
// 		}
// 	} else {
// 		depositStr, err = flags.GetString(FlagDeposit)
// 		if err != nil {
// 			return sdk.Coin{}, err
// 		}
// 	}
//
// 	deposit, err = sdk.ParseCoinNormalized(depositStr)
// 	if err != nil {
// 		return sdk.Coin{}, err
// 	}
//
// 	return deposit, nil
// }

func MarkReqDepositFlags(cmd *cobra.Command) {
	_ = cmd.MarkFlagRequired(FlagDeposit)
}
