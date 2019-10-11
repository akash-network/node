package initgen_test

import (
	"testing"

	"github.com/ovrclk/akash/util/initgen"
	"github.com/stretchr/testify/require"
)

func BuilderTestSingle(t *testing.T) {
	ctx, err := initgen.NewBuilder().
		WithNames([]string{"foo"}).
		WithPath("/baz").
		Create()

	require.NoError(t, err)

	require.Len(t, ctx.Nodes(), 1)
	require.Len(t, ctx.Genesis().Validators, 1)
	require.Equal(t, "foo", ctx.Genesis().Validators[0])
	require.Equal(t, "/baz", ctx.Path())
}

func BuilderTestMulti(t *testing.T) {
	ctx, err := initgen.NewBuilder().
		WithNames([]string{"foo", "bar"}).
		WithPath("/baz").
		Create()

	require.NoError(t, err)

	require.Len(t, ctx.Nodes(), 2)
	require.Len(t, ctx.Genesis().Validators, 2)
	require.Equal(t, "foo", ctx.Genesis().Validators[0])
	require.Equal(t, "bar", ctx.Genesis().Validators[1])
	require.Equal(t, "/baz", ctx.Path())
}
