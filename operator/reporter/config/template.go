package config

import (
	"bytes"
	"fmt"
	"strconv"
	"text/template"
	"time"
)

const fileTemplate = `# akt-reporter configuration.
# Every key can also be set with a flag (--<key>) or an AKT_REPORTER_<KEY> environment variable
# ("." and "-" become "_"). Precedence: flag, environment, this file, default.

{{c "network"}}
network = {{s .Network}}

{{c "broadcast"}}
broadcast = {{.Broadcast}}

{{c "reporter"}}
reporter = {{s .Reporter}}

{{c "chain-id"}}
chain-id = {{s .ChainID}}

{{c "node"}}
node = {{s .Node}}

{{c "grpc-addr"}}
grpc-addr = {{s .GRPCAddr}}

{{c "grpc-insecure"}}
grpc-insecure = {{.GRPCInsecure}}

{{c "max-age"}}
max-age = {{d .MaxAge}}

{{c "request-timeout"}}
request-timeout = {{d .RequestTimeout}}

{{c "from"}}
from = {{s .From}}

{{c "keyring-backend"}}
keyring-backend = {{s .KeyringBackend}}

{{c "keyring-dir"}}
keyring-dir = {{s .KeyringDir}}

{{c "gas"}}
gas = {{s .Gas}}

{{c "gas-adjustment"}}
gas-adjustment = {{f .GasAdjustment}}

{{c "gas-prices"}}
gas-prices = {{s .GasPrices}}

{{c "fees"}}
fees = {{s .Fees}}

[solana]
{{c "solana.rpc"}}
rpc = {{s .Solana.RPC}}

{{c "solana.genesis-hash"}}
genesis-hash = {{s .Solana.GenesisHash}}

{{c "solana.program-id"}}
program-id = {{s .Solana.ProgramID}}

{{c "solana.mint"}}
mint = {{s .Solana.Mint}}

{{c "solana.light-client-program"}}
light-client-program = {{s .Solana.LightClientProgram}}

{{c "solana.max-slot-lead"}}
max-slot-lead = {{.Solana.MaxSlotLead}}

[scan]
{{c "scan.interval"}}
interval = {{d .Scan.Interval}}

{{c "scan.audit-per-scan"}}
audit-per-scan = {{.Scan.AuditPerScan}}

{{c "scan.page-size"}}
page-size = {{.Scan.PageSize}}

{{c "scan.workers"}}
workers = {{.Scan.Workers}}

{{c "scan.akash-concurrency"}}
akash-concurrency = {{.Scan.AkashConcurrency}}

{{c "scan.solana-concurrency"}}
solana-concurrency = {{.Scan.SolanaConcurrency}}

{{c "scan.block-concurrency"}}
block-concurrency = {{.Scan.BlockConcurrency}}

[tx]
{{c "tx.max-gas"}}
max-gas = {{.Tx.MaxGas}}

{{c "tx.max-fee"}}
max-fee = {{s .Tx.MaxFee}}

{{c "tx.max-msgs"}}
max-msgs = {{.Tx.MaxMsgs}}

{{c "tx.max-in-flight"}}
max-in-flight = {{.Tx.MaxInFlight}}

[signer.remote]
{{c "signer.remote.address"}}
address = {{s .Signer.Remote.Address}}

{{c "signer.remote.key-id"}}
key-id = {{s .Signer.Remote.KeyID}}

{{c "signer.remote.ca-file"}}
ca-file = {{s .Signer.Remote.CAFile}}

{{c "signer.remote.server-name"}}
server-name = {{s .Signer.Remote.ServerName}}

{{c "signer.remote.cert-file"}}
cert-file = {{s .Signer.Remote.CertFile}}

{{c "signer.remote.key-file"}}
key-file = {{s .Signer.Remote.KeyFile}}

{{c "signer.remote.timeout"}}
timeout = {{d .Signer.Remote.Timeout}}

[metrics]
{{c "metrics.listen"}}
listen = {{s .Metrics.Listen}}
`

var parsedTemplate = template.Must(template.New("reporter.toml").Funcs(template.FuncMap{
	"c": func(key string) (string, error) {
		usage, ok := usages[key]
		if !ok {
			return "", fmt.Errorf("no usage for key %s", key)
		}

		return "# " + usage, nil
	},
	"s": strconv.Quote,
	"d": func(d time.Duration) string { return strconv.Quote(d.String()) },
	"f": func(f float64) string {
		s := strconv.FormatFloat(f, 'g', -1, 64)
		for _, ch := range s {
			if ch == '.' || ch == 'e' || ch == 'E' {
				return s
			}
		}

		return s + ".0"
	},
}).Parse(fileTemplate))

// Render returns the reporter.toml text of c, with one comment per key.
func Render(c Config) ([]byte, error) {
	var buf bytes.Buffer
	if err := parsedTemplate.Execute(&buf, c); err != nil {
		return nil, fmt.Errorf("render configuration: %w", err)
	}

	return buf.Bytes(), nil
}
