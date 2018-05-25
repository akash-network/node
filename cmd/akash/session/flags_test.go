package session_test

import (
	"os"
	"strconv"
	"testing"

	"github.com/ovrclk/akash/cmd/akash/constants"
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/testutil"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/go-crypto/keys"
)

type flagFn func(cmd *cobra.Command, flags *pflag.FlagSet)

func TestNode_Env(t *testing.T) {
	const val = "foo.bar:123"
	defer os.Unsetenv("AKASH_NODE")

	os.Setenv("AKASH_NODE", val)

	assertCommand(t, session.AddFlagNode, func(sess session.Session, cmd *cobra.Command, args []string) error {
		assert.Equal(t, val, sess.Node())
		return nil
	})
}

func TestNode_Flag(t *testing.T) {
	const val = "foo.bar:123"

	assertCommand(t, session.AddFlagNode, func(sess session.Session, cmd *cobra.Command, args []string) error {
		assert.Equal(t, val, sess.Node())
		return nil
	}, "-n", val)
}

func TestKey_Flag(t *testing.T) {
	const val = "foo"

	assertCommand(t, session.AddFlagKey, func(sess session.Session, cmd *cobra.Command, args []string) error {
		assert.Equal(t, val, sess.KeyName())
		return nil
	}, "-k", val)
}

func TestFlag_Nonce(t *testing.T) {
	const val uint64 = 10
	const key = "foo"

	flagfn := func(cmd *cobra.Command, flags *pflag.FlagSet) {
		session.AddFlagKey(cmd, flags)
		session.AddFlagNonce(cmd, flags)
		session.AddFlagNode(cmd, flags)
	}

	assertCommand(t, flagfn, func(sess session.Session, cmd *cobra.Command, args []string) error {

		kmgr, err := sess.KeyManager()
		require.NoError(t, err)

		_, _, err = kmgr.Create(key, constants.Password, keys.AlgoEd25519)
		require.NoError(t, err)

		nonce, err := sess.Nonce()
		require.NoError(t, err)
		require.Equal(t, val, nonce)
		return nil
	}, "--"+constants.FlagNonce, strconv.Itoa(int(val)), "-k", key, "-n", "node.address")
}

func assertCommand(t *testing.T, flagfn flagFn, fn session.Runner, args ...string) {
	testutil.WithAkashDir(t, func(basedir string) {
		viper.Reset()

		ran := false

		cmd := &cobra.Command{
			Use: "test",
			RunE: session.WithSession(func(sess session.Session, cmd *cobra.Command, args []string) error {
				ran = true
				require.Equal(t, basedir, sess.RootDir(), "unexpected home dir")
				return fn(sess, cmd, args)
			}),
		}

		session.SetupBaseCommand(cmd)

		if flagfn != nil {
			flagfn(cmd, cmd.Flags())
		}

		cmd.SetArgs(args)
		require.NoError(t, cmd.Execute())
		assert.True(t, ran)

	})
}
