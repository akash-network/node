package operatorcommon

import (
	"github.com/stretchr/testify/require"
	"io"
	"testing"
)

func TestOperatorServer(t *testing.T) {
	server, err := NewOperatorHTTP()
	require.NoError(t, err)
	require.NotNil(t, server)

	require.NotNil(t, server.GetRouter())

	called := false
	flag := server.AddPreparedEndpoint("/thepath", func(pd PreparedResult) error {
		require.NotNil(t, pd)
		pd.Set([]byte{0x0, 0x1, 0x2})
		called = true
		return nil
	})

	require.NoError(t, server.PrepareAll())
	require.False(t, called)

	flag()

	require.NoError(t, server.PrepareAll())
	require.True(t, called)
}

func TestOperatorServerReturnsPrepareError(t *testing.T) {
	server, err := NewOperatorHTTP()
	require.NoError(t, err)
	require.NotNil(t, server)

	flag := server.AddPreparedEndpoint("/thepath", func(_ PreparedResult) error {
		return io.EOF
	})
	flag()
	require.ErrorIs(t, server.PrepareAll(), io.EOF)
}

func TestOperatorServerPanicsOnDuplicatePath(t *testing.T) {
	server, err := NewOperatorHTTP()
	require.NoError(t, err)
	require.NotNil(t, server)

	_ = server.AddPreparedEndpoint("/thepath", func(_ PreparedResult) error { return nil })
	require.PanicsWithValue(t, "prepared result exists for path: /thepath", func() {
		_ = server.AddPreparedEndpoint("/thepath", func(_ PreparedResult) error { return nil })
	})

	require.PanicsWithValue(t, "passed nil value for prepare function", func() {
		_ = server.AddPreparedEndpoint("/lhjkbhjlkb", nil)
	})
}
