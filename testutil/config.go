package testutil

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TempDir(t *testing.T) string {
	basedir, err := ioutil.TempDir("", t.Name())
	require.NoError(t, err)
	return basedir
}

func WithTempDir(t *testing.T, fn func(string)) {
	dir := TempDir(t)
	defer os.RemoveAll(dir)
	fn(dir)
}

func WithTempDirEnv(t *testing.T, key string, fn func(string)) {
	WithTempDir(t, func(dir string) {
		// XXX: not thread/parallel-test safe
		prev := os.Getenv(key)
		os.Setenv(key, dir)
		defer os.Setenv(key, prev)
		fn(dir)
	})
}

func WithAkashDir(t *testing.T, fn func(string)) {
	WithTempDirEnv(t, "AKASH_DATA", fn)
}
