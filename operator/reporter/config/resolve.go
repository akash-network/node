package config

import (
	"cmp"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/gagliardetto/solana-go"

	aktsolana "pkg.akt.dev/node/v2/operator/reporter/solana"
)

const (
	uaktDenom      = "uakt"
	networkMainnet = "mainnet"
	networkTestnet = "testnet"

	// maxBroadcastBudget is max-age plus the signer timeout in broadcast mode (spec section 6.3).
	maxBroadcastBudget = 80 * time.Second
	// slotLeadMargin is added to max-age when deriving the slot lead, at 5 slots per second.
	slotLeadMargin   = 60 * time.Second
	slotsPerSecond   = 5
	maxFeeUakt       = 1_000_000
	maxTxGas         = 20_000_000
	maxSlotLeadLimit = 100_000
)

var (
	uaktAmount = regexp.MustCompile(`^(0|[1-9][0-9]*)uakt$`)
	gasAmount  = regexp.MustCompile(`^[1-9][0-9]*$`)
)

// Resolved is a validated Config with its derived values parsed.
type Resolved struct {
	Config

	ProgramID, Mint solana.PublicKey
	// LightClientPin is nil when solana.light-client-program is empty, which follows the bridge account.
	LightClientPin *solana.PublicKey
	Reporter       sdk.AccAddress
	// SlotLead is solana.max-slot-lead, or (max-age + 60 s) x 5 when that is 0.
	SlotLead uint64
	// GasPerMessage is 0 for gas = auto.
	GasPerMessage uint64
	// GasPrices is empty when only fees is set.
	GasPrices sdk.DecCoins
	// Fees is empty when fees is not set.
	Fees   sdk.Coins
	MaxFee sdk.Coin
}

type rules struct {
	errs []error
}

func (r *rules) add(key, format string, args ...any) {
	r.errs = append(r.errs, fmt.Errorf("%s: %s", key, fmt.Sprintf(format, args...)))
}

func between[T cmp.Ordered](r *rules, key string, v, lo, hi T) {
	// cmp.Compare orders a NaN below every number, so a NaN fails the check.
	if cmp.Compare(v, lo) < 0 || cmp.Compare(v, hi) > 0 {
		r.add(key, "must be between %v and %v, got %v", lo, hi, v)
	}
}

// checkURL never echoes the value: it may hold credentials.
func (r *rules) checkURL(key, raw string, schemes ...string) {
	if raw == "" {
		r.add(key, "is required")

		return
	}

	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		r.add(key, "is not a valid URL")

		return
	}

	if !slices.Contains(schemes, u.Scheme) {
		r.add(key, "scheme must be one of %v, got %q", schemes, u.Scheme)
	}

	if u.User != nil {
		r.add(key, "use a local authenticated proxy instead of credentials in RPC URLs")
	}
}

func (r *rules) checkHostPort(key, v string) {
	host, port, err := net.SplitHostPort(v)
	if err != nil || host == "" {
		r.add(key, "must be host:port, got %q", v)

		return
	}

	if n, err := strconv.ParseUint(port, 10, 16); err != nil || n == 0 {
		r.add(key, "invalid port in %q", v)
	}
}

func (r *rules) publicKey(key, s string) (solana.PublicKey, bool) {
	if s == "" {
		r.add(key, "is required")

		return solana.PublicKey{}, false
	}

	pk, err := solana.PublicKeyFromBase58(s)
	if err != nil {
		r.add(key, "must be a base58 public key of 32 bytes")

		return solana.PublicKey{}, false
	}

	if pk.String() != s {
		r.add(key, "noncanonical base58")

		return solana.PublicKey{}, false
	}

	return pk, true
}

func parseUakt(s string) (sdk.Coin, error) {
	m := uaktAmount.FindStringSubmatch(s)
	if m == nil {
		return sdk.Coin{}, errors.New("must be a whole amount of uakt, for example 1000000uakt")
	}

	n, ok := sdkmath.NewIntFromString(m[1])
	if !ok {
		return sdk.Coin{}, errors.New("amount is too large")
	}

	return sdk.Coin{Denom: uaktDenom, Amount: n}, nil
}

// Resolve checks every rule of spec sections 8.3 and 8.4 and parses the derived values. The error joins every
// broken rule, each starting with its key. The Resolved is zero when there is an error.
func Resolve(c Config) (Resolved, error) {
	var (
		r   rules
		out = Resolved{Config: c}
	)

	if c.Network != networkMainnet && c.Network != networkTestnet {
		r.add("network", "must be mainnet or testnet, got %q", c.Network)
	}

	r.reporter(&out)

	if c.ChainID == "" {
		r.add("chain-id", "is required")
	}

	r.checkURL("node", c.Node, "tcp", "http", "https")
	r.checkHostPort("grpc-addr", c.GRPCAddr)

	if c.MaxAge <= 0 || c.MaxAge > 10*time.Minute {
		r.add("max-age", "must be above 0 and at most 10m0s, got %v", c.MaxAge)
	}

	between(&r, "request-timeout", c.RequestTimeout, time.Second, time.Minute)

	switch c.KeyringBackend {
	case "os", "file", "test":
	default:
		r.add("keyring-backend", "must be os, file or test, got %q", c.KeyringBackend)
	}

	between(&r, "gas-adjustment", c.GasAdjustment, 1.0, 10.0)

	between(&r, "tx.max-gas", c.Tx.MaxGas, 1, maxTxGas)
	between(&r, "tx.max-msgs", c.Tx.MaxMsgs, 1, 100)
	between(&r, "tx.max-in-flight", c.Tx.MaxInFlight, 1, 64)
	r.maxFee(&out)
	r.gas(&out)
	r.fees(&out)

	r.solana(&out)

	between(&r, "scan.interval", c.Scan.Interval, time.Second, time.Minute)
	between(&r, "scan.audit-per-scan", c.Scan.AuditPerScan, 1, 10_000)
	between(&r, "scan.page-size", c.Scan.PageSize, 1, 5_000)
	between(&r, "scan.workers", c.Scan.Workers, 1, 256)
	between(&r, "scan.akash-concurrency", c.Scan.AkashConcurrency, 1, 256)
	between(&r, "scan.solana-concurrency", c.Scan.SolanaConcurrency, 1, 64)
	between(&r, "scan.block-concurrency", c.Scan.BlockConcurrency, 1, 16)

	r.signer(c)

	if c.Metrics.Listen != "" {
		r.checkHostPort("metrics.listen", c.Metrics.Listen)
	}

	r.networkGuard(c)

	if err := errors.Join(r.errs...); err != nil {
		return Resolved{}, err
	}

	lead := c.Solana.MaxSlotLead
	if lead == 0 {
		lead = uint64( //nolint:gosec // MaxAge is above 0 here
			(c.MaxAge+slotLeadMargin+time.Second-1)/time.Second) * slotsPerSecond
	}

	out.SlotLead = lead

	return out, nil
}

func (r *rules) reporter(out *Resolved) {
	if out.Config.Reporter == "" {
		r.add("reporter", "is required")

		return
	}

	hrp, raw, err := bech32.DecodeAndConvert(out.Config.Reporter)
	if err == nil {
		var again string
		again, err = bech32.ConvertAndEncode(hrp, raw)

		if err == nil && (hrp != "akash" || len(raw) != 20 || again != out.Config.Reporter) {
			err = errors.New("noncanonical")
		}
	}

	if err != nil {
		r.add("reporter", "invalid reporter address: must be a canonical lowercase akash1... address of 20 bytes")

		return
	}

	out.Reporter = sdk.AccAddress(raw)
}

func (r *rules) maxFee(out *Resolved) {
	coin, err := parseUakt(out.Tx.MaxFee)
	if err != nil {
		r.add("tx.max-fee", "%v", err)

		return
	}

	switch {
	case !coin.Amount.IsPositive():
		r.add("tx.max-fee", "must be above 0, got %s", out.Tx.MaxFee)

		return
	case coin.Amount.GT(sdkmath.NewInt(maxFeeUakt)):
		r.add("tx.max-fee", "must be at most %duakt, got %s", maxFeeUakt, out.Tx.MaxFee)

		return
	}

	out.MaxFee = coin
}

func (r *rules) gas(out *Resolved) {
	if out.Gas == "auto" {
		return
	}

	if !gasAmount.MatchString(out.Gas) {
		r.add("gas", "must be auto or a positive integer of gas per message, got %q", out.Gas)

		return
	}

	gas, err := strconv.ParseUint(out.Gas, 10, 64)
	if err != nil || gas > out.Tx.MaxGas {
		r.add("gas", "must be at most tx.max-gas (%d), got %s", out.Tx.MaxGas, out.Gas)

		return
	}

	out.GasPerMessage = gas
}

func (r *rules) fees(out *Resolved) {
	if out.Config.Fees != "" {
		coin, err := parseUakt(out.Config.Fees)

		switch {
		case err != nil:
			r.add("fees", "%v", err)
		case !coin.Amount.IsPositive():
			r.add("fees", "must be above 0, got %s", out.Config.Fees)
		case out.MaxFee.Amount.IsNil():
			// tx.max-fee is already reported
		case coin.Amount.GT(out.MaxFee.Amount):
			r.add("fees", "must be at most tx.max-fee (%s), got %s", out.Tx.MaxFee, out.Config.Fees)
		default:
			out.Fees = sdk.NewCoins(coin)
		}
	}

	if out.Config.GasPrices == "" {
		if out.Config.Fees == "" {
			r.add("gas-prices", "is required when fees is empty")
		}

		return
	}

	prices, err := sdk.ParseDecCoins(out.Config.GasPrices)
	if err != nil || len(prices) != 1 || prices[0].Denom != uaktDenom {
		r.add("gas-prices", "must be a single positive uakt price, for example 0.025uakt")

		return
	}

	out.GasPrices = prices
}

func (r *rules) solana(out *Resolved) {
	s := out.Solana

	r.checkURL("solana.rpc", s.RPC, "http", "https")

	r.publicKey("solana.genesis-hash", s.GenesisHash)

	program, programOK := r.publicKey("solana.program-id", s.ProgramID)
	mint, mintOK := r.publicKey("solana.mint", s.Mint)

	if programOK {
		out.ProgramID = program
	}

	if mintOK {
		out.Mint = mint
	}

	if programOK && mintOK && mint != aktsolana.PDA(program, []byte("mint")) {
		r.add("solana.mint", "noncanonical mint: must equal the \"mint\" PDA of solana.program-id")
	}

	if s.LightClientProgram != "" {
		if pk, ok := r.publicKey("solana.light-client-program", s.LightClientProgram); ok {
			out.LightClientPin = &pk
		}
	}

	between(r, "solana.max-slot-lead", s.MaxSlotLead, 0, maxSlotLeadLimit)
}

func (r *rules) signer(c Config) {
	remote := c.Signer.Remote

	between(r, "signer.remote.timeout", remote.Timeout, time.Second, 120*time.Second)

	if remote.Address != "" {
		r.checkHostPort("signer.remote.address", remote.Address)

		if remote.KeyID == "" {
			r.add("signer.remote.key-id", "is required with signer.remote.address")
		}
	}

	if (remote.CertFile == "") != (remote.KeyFile == "") {
		r.add("signer.remote.cert-file", "set signer.remote.cert-file and signer.remote.key-file together or not at all")
	}

	if !c.Broadcast {
		return
	}

	var signerTimeout time.Duration

	switch {
	case c.From != "" && remote.Address != "":
		r.add("from", "configure from or signer.remote.address, not both")
	case c.From == "" && remote.Address == "":
		r.add("from", "broadcast needs exactly one of from and signer.remote.address")
	}

	if remote.Address != "" {
		signerTimeout = remote.Timeout
	}

	if c.MaxAge+signerTimeout > maxBroadcastBudget {
		if remote.Address != "" {
			r.add("max-age", "max-age (%v) plus signer.remote.timeout (%v) must be at most %v when broadcasting", c.MaxAge, signerTimeout, maxBroadcastBudget)
		} else {
			r.add("max-age", "max-age (%v) must be at most %v when broadcasting with the keyring signer (from)", c.MaxAge, maxBroadcastBudget)
		}
	}
}

func (r *rules) networkGuard(c Config) {
	switch c.Network {
	case networkMainnet:
		if c.ChainID != "" && c.ChainID != MainnetChainID {
			r.add("network", "mainnet requires chain-id %s, got %q", MainnetChainID, c.ChainID)
		}

		if c.Solana.GenesisHash != "" && c.Solana.GenesisHash != MainnetGenesis {
			r.add("network", "mainnet requires solana.genesis-hash %s, got %q", MainnetGenesis, c.Solana.GenesisHash)
		}

		if c.Solana.ProgramID == DevProgramID {
			r.add("network", "mainnet refuses the development program ID %s", DevProgramID)
		}
	case networkTestnet:
		if c.ChainID == MainnetChainID {
			r.add("network", "testnet refuses the mainnet chain-id %s", MainnetChainID)
		}

		if c.Solana.GenesisHash == MainnetGenesis {
			r.add("network", "testnet refuses the mainnet solana.genesis-hash %s", MainnetGenesis)
		}
	}
}
