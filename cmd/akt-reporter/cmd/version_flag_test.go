package cmd_test

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/cosmos/cosmos-sdk/version"
	"github.com/stretchr/testify/require"
)

func TestPackagedVersionFlag(t *testing.T) {
	rawCLIHome(t)
	want := fmt.Sprintf("service akt-reporter\nversion %s\ncommit %s\nplatform %s_%s\ngo %s\n",
		version.Version, version.Commit, runtime.GOOS, runtime.GOARCH, runtime.Version())
	for _, args := range [][]string{{"--version"}, {"version"}} {
		code, stdout, stderr := execute(t, "", args...)
		require.Equal(t, 0, code, stderr)
		require.Equal(t, want, stdout)
		require.Empty(t, stderr)
	}
}

func TestVersionFlagPreservesLoggingAndHelp(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"version"}} {
		code, stdout, stderr := execute(t, "", append(args, "--log_level", "invalid")...)
		require.Equal(t, 1, code)
		require.Empty(t, stdout)
		require.Contains(t, stderr, "invalid log_level")
	}

	for _, args := range [][]string{nil, {"--help"}, {"--version=false"}} {
		code, stdout, stderr := execute(t, "", args...)
		require.Equal(t, 0, code, stderr)
		require.Contains(t, stdout, "Usage:")
		require.Contains(t, stdout, "--version")
	}
}
