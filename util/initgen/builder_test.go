package initgen_test

import (
	"testing"

	"github.com/ovrclk/akash/util/initgen"
	"github.com/stretchr/testify/require"
)

func BuilderTestSingle(t *testing.T) {
	ctx, err := initgen.NewBuilder().
		WithName("foo").
		WithCount(1).
		WithPath("/bar").
		Create()

	require.NoError(t, err)

	require.Len(t, ctx.Nodes(), 1)
	require.Len(t, ctx.Genesis().Validators, 1)
	require.Equal(t, "foo", ctx.Genesis().Validators[0])
	require.Equal(t, "foo", ctx.Name())
	require.Equal(t, "/bar", ctx.Path())
}

func BuilderTestMulti(t *testing.T) {
	ctx, err := initgen.NewBuilder().
		WithName("foo").
		WithCount(5).
		WithPath("/bar").
		Create()

	require.NoError(t, err)

	require.Len(t, ctx.Nodes(), 5)
	require.Len(t, ctx.Genesis().Validators, 5)
	require.Equal(t, "foo-0", ctx.Genesis().Validators[0])
	require.Equal(t, "foo-4", ctx.Genesis().Validators[4])
	require.Equal(t, "foo", ctx.Name())
	require.Equal(t, "/bar", ctx.Path())
}
