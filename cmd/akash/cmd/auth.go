package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/cobra"

	sdkclient "github.com/cosmos/cosmos-sdk/client"

	ajwt "pkg.akt.dev/go/util/jwt"
)

const (
	FlagJWTExp    = "exp"
	FlagJWTNbf    = "nbf"
	FlagJWTAccess = "access"
	FlagJWTScope  = "scope"
)

func AuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "auth",
	}

	cmd.AddCommand(authJWTCmd())

	return cmd
}

func authJWTCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "jwt",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cctx, err := sdkclient.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			signer := ajwt.NewSigner(cctx.Keyring, cctx.FromAddress)

			now := time.Now()
			nbf := now

			expString, err := cmd.Flags().GetString(FlagJWTExp)
			if err != nil {
				return err
			}

			var exp time.Time
			// first, attempt to parse expiration value as duration.
			// fallback to unix timestamp if fails
			dur, err := time.ParseDuration(expString)
			if err != nil {
				expInt, err := strconv.ParseInt(expString, 10, 64)
				if err != nil {
					return err
				}

				exp = time.Unix(expInt, 0)
			} else {
				exp = now.Add(dur)
			}

			if cmd.Flags().Changed(FlagJWTNbf) {
				nbfString, err := cmd.Flags().GetString(FlagJWTNbf)
				if err != nil {
					return err
				}

				// first, attempt to parse expiration value as duration.
				// fallback to unix timestamp if fails
				dur, err := time.ParseDuration(nbfString)
				if err != nil {
					nbfInt, err := strconv.ParseInt(nbfString, 10, 64)
					if err != nil {
						return err
					}

					exp = time.Unix(nbfInt, 0)
				} else {
					exp = now.Add(dur)
				}
			}

			accessString, err := cmd.Flags().GetString(FlagJWTAccess)
			if err != nil {
				return err
			}

			access, valid := parseAccess(accessString)
			if !valid {
				return fmt.Errorf("invalid `access` flags")
			}

			var scope ajwt.PermissionScopes

			if cmd.Flags().Changed(FlagJWTScope) {
				scopeString, err := cmd.Flags().GetString(FlagJWTAccess)
				if err != nil {
					return err
				}

				if err = scope.UnmarshalCSV(scopeString); err != nil {
					return err
				}
			}

			if !exp.After(now) {
				return fmt.Errorf("`exp` value is invalid or in the past. expected %d (exp) > %d (curr)", exp.Unix(), now.Unix())
			}

			if !nbf.After(exp) {
				return fmt.Errorf("`nbf` value is invalid. expected %d (nbf) < %d (exp)", nbf.Unix(), exp.Unix())
			}

			claims := ajwt.Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Issuer:    cctx.FromAddress.String(),
					IssuedAt:  jwt.NewNumericDate(now),
					NotBefore: jwt.NewNumericDate(nbf),
					ExpiresAt: jwt.NewNumericDate(exp),
				},
				Version: "v1",
				Leases: ajwt.Leases{
					Access: access,
					Scope:  scope,
				},
			}

			tok := jwt.NewWithClaims(ajwt.SigningMethodES256K, &claims)

			tokString, err := tok.SignedString(signer)
			if err != nil {
				return err
			}

			return cctx.PrintString(tokString)
		},
	}

	cmd.Flags().String(FlagJWTExp, "15m", "Set token's `exp` field. Format is either 15m|h or unix timestamp")
	cmd.Flags().String(FlagJWTNbf, "", "Set token's `nbf` field. Format is either 15m|h or unix timestamp. Empty equals to current timestamp")
	cmd.Flags().String(FlagJWTAccess, "full", "Set token's `leases.access` field. Permitted values are full|scoped|granular. Default is full")
	cmd.Flags().StringSlice(FlagJWTScope, nil, fmt.Sprintf("Set token's `leases.scope` field. Comma separated list of scopes. Can only be set if `leases.access=scoped`. Allowed scopes are (%s)", strings.Join(permissionScopesToStrings(ajwt.GetSupportedScopes()), "|")))

	return cmd
}

func parseAccess(val string) (ajwt.AccessType, bool) {
	res := ajwt.AccessType(val)

	switch res {
	case ajwt.AccessTypeFull:
	case ajwt.AccessTypeScoped:
	case ajwt.AccessTypeGranular:
	default:
		return ajwt.AccessTypeNone, false
	}

	return res, true
}

func permissionScopesToStrings(scopes ajwt.PermissionScopes) []string {
	result := make([]string, len(scopes))
	for i, scope := range scopes {
		result[i] = string(scope)
	}
	return result
}
