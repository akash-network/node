package session

import (
	"os"
	"strconv"
	"testing"

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

	assertCommand(t, AddFlagNode, func(sess Session, cmd *cobra.Command, args []string) error {
		assert.Equal(t, val, sess.Node())
		return nil
	})
}

func TestNode_Flag(t *testing.T) {
	const val = "foo.bar:123"

	assertCommand(t, AddFlagNode, func(sess Session, cmd *cobra.Command, args []string) error {
		assert.Equal(t, val, sess.Node())
		return nil
	}, "-n", val)
}

func TestKey_Flag(t *testing.T) {
	const val = "foo"

	assertCommand(t, AddFlagKey, func(sess Session, cmd *cobra.Command, args []string) error {
		assert.Equal(t, val, sess.KeyName())
		return nil
	}, "-k", val)
}

func TestFlag_Nonce(t *testing.T) {
	const val uint64 = 10
	const key = "foo"

	flagfn := func(cmd *cobra.Command, flags *pflag.FlagSet) {
		AddFlagKey(cmd, flags)
		AddFlagNonce(cmd, flags)
		AddFlagNode(cmd, flags)
	}

	assertCommand(t, flagfn, func(sess Session, cmd *cobra.Command, args []string) error {

		kmgr, err := sess.KeyManager()
		require.NoError(t, err)

		_, _, err = kmgr.Create(key, password, keys.AlgoEd25519)
		require.NoError(t, err)

		nonce, err := sess.Nonce()
		require.NoError(t, err)
		require.Equal(t, val, nonce)
		return nil
	}, "--"+flagNonce, strconv.Itoa(int(val)), "-k", key, "-n", "node.address")
}

func assertCommand(t *testing.T, flagfn flagFn, fn Runner, args ...string) {
	testutil.WithAkashDir(t, func(basedir string) {
		viper.Reset()

		ran := false

		cmd := &cobra.Command{
			Use: "test",
			RunE: WithSession(func(sess Session, cmd *cobra.Command, args []string) error {
				ran = true
				require.Equal(t, basedir, sess.RootDir(), "unexpected home dir")
				return fn(sess, cmd, args)
			}),
		}

		SetupBaseCommand(cmd)

		if flagfn != nil {
			flagfn(cmd, cmd.Flags())
		}

		cmd.SetArgs(args)
		require.NoError(t, cmd.Execute())
		assert.True(t, ran)

	})
}
