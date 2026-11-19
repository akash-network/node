package cmd_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const smokeEnvFile = "../../../_build/akt-reporter-smoke.env"

// TestSmokeEnvPassesCheck guards the environment the release workflow gives to `check --network testnet` in the image.
func TestSmokeEnvPassesCheck(t *testing.T) {
	data, err := os.ReadFile(smokeEnvFile)
	require.NoError(t, err)

	// docker --env-file takes the text after "=" literally, so the file may hold only plain KEY=value lines.
	seen := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		name, value, ok := strings.Cut(line, "=")
		require.True(t, ok, line)
		require.True(t, strings.HasPrefix(name, "AKT_REPORTER_"), line)
		require.NotEmpty(t, value, line)
		require.NotContains(t, value, `"`, line)
		require.NotContains(t, value, "'", line)

		t.Setenv(name, value)
		seen++
	}

	require.NotZero(t, seen)

	home := t.TempDir()
	t.Setenv("HOME", home)

	code, stdout, stderr := execute(t, "", "check", "--home", home, "--network", "testnet")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, "Configuration valid. No RPC request or transaction sent.\n", stdout)
}
