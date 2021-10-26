package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"net/url"

	"testing"
)

var expectedErrMsgForRPC = "^error communicating with RPC.+$"

func TestUnwrappingRPCJSONError(t *testing.T) {
	buf := &bytes.Buffer{}
	buf.WriteString("{foo:bar}") // some invalid json
	dec := json.NewDecoder(buf)
	var x interface{}
	err := dec.Decode(&x)
	require.Error(t, err)
	require.IsType(t, &json.SyntaxError{}, err)

	wrappedErr := fmt.Errorf("%w: test error", err)
	err = markRPCServerError(wrappedErr)
	require.Error(t, err)
	require.Regexp(t, expectedErrMsgForRPC, err)
}

func TestUnwrappingURLError(t *testing.T) {
	urlErr := &url.Error{
		Op:  "GET",
		URL: "a",
		Err: errors.New("test error thing"),
	}
	require.Error(t, urlErr)

	wrappedErr := fmt.Errorf("%w: test error", urlErr)
	err := markRPCServerError(wrappedErr)
	require.Error(t, err)
	require.Regexp(t, expectedErrMsgForRPC, err)
}
