package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/operator/reporter/config"
	"pkg.akt.dev/node/v2/operator/reporter/signer"
	"pkg.akt.dev/node/v2/operator/reporter/tx"
)

func TestCommandPackagedCheckAndFlags(t *testing.T) {
	path := legacyCLIFile(t, "http://127.0.0.1:1")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var input map[string]any
	require.NoError(t, json.Unmarshal(data, &input))
	delete(input, "keyFile")
	input["externalSigner"] = map[string]any{
		"command": "/nonexistent/r196/never-spawned",
	}
	data, err = json.Marshal(input)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))
	for _, broadcast := range []bool{false, true} {
		args := []string{"check", "--config", path, "--network", "testnet"}
		if broadcast {
			args = append(args, "--broadcast")
		}
		root := NewRootCmd()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetArgs(args)
		require.NoError(t, root.ExecuteContext(t.Context()))
		require.Equal(t, checkValid+"\n", out.String())
		for _, flag := range []string{"--key-file", "--signer-command"} {
			for _, command := range []string{"run", "check"} {
				root := NewRootCmd()
				root.SetArgs([]string{command, "--config", path, "--network", "testnet", flag, "/forbidden"})
				require.ErrorContains(t, root.ExecuteContext(t.Context()), "unknown flag")
			}
		}
	}
}

func TestCommandSignerTimeoutCutoff(t *testing.T) {
	c := config.Default()
	c.Legacy = &config.Legacy{
		ExternalSigner: &config.ExternalSigner{
			Options: signer.CommandOptions{
				Command: "/not-run",
				Timeout: 17 * time.Second,
			},
		},
	}
	r := &reporter{
		cfg: config.Resolved{
			Config: c,
		},
	}
	require.Equal(t, 17*time.Second, r.signerTimeout())
	require.Equal(t, 37*time.Second, tx.SendCutoff(r.signerTimeout()))
	c.Legacy.ExternalSigner = nil
	require.Zero(t, r.signerTimeout())
	r.cfg.Legacy = nil
	r.cfg.From = "local"
	require.Zero(t, r.signerTimeout())
	r.cfg.From = ""
	r.cfg.Signer.Remote.Timeout = 8 * time.Second
	require.Equal(t, 8*time.Second, r.signerTimeout())
}
