package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec/legacy"
)

type errorResponse struct {
	Code  int    `json:"code,omitempty"`
	Error string `json:"error"`
}

// newErrorResponse creates a new errorResponse instance.
func newErrorResponse(code int, err string) errorResponse {
	return errorResponse{Code: code, Error: err}
}

// writeErrorResponse prepares and writes a HTTP error
// given a status code and an error message.
func WriteErrorResponse(w http.ResponseWriter, status int, err string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(legacy.Cdc.MustMarshalJSON(newErrorResponse(0, err)))
}

// PostProcessResponse performs post processing for a REST response. The result
// returned to clients will contain two fields, the height at which the resource
// was queried at and the original result.
func PostProcessResponse(w http.ResponseWriter, ctx client.Context, resp interface{}) {
	var (
		result []byte
		err    error
	)

	if ctx.Height < 0 {
		WriteErrorResponse(w, http.StatusInternalServerError, fmt.Errorf("negative height in response").Error())
		return
	}

	// LegacyAmino used intentionally for REST
	marshaler := ctx.LegacyAmino

	switch res := resp.(type) {
	case []byte:
		result = res

	default:
		result, err = marshaler.MarshalJSON(resp)
		if CheckInternalServerError(w, err) {
			return
		}
	}

	wrappedResp := NewResponseWithHeight(ctx.Height, result)

	output, err := marshaler.MarshalJSON(wrappedResp)
	if CheckInternalServerError(w, err) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(output)
}

// CheckInternalServerError attaches an error message to an HTTP 500 INTERNAL SERVER ERROR response.
// Returns false when err is nil; it returns true otherwise.
func CheckInternalServerError(w http.ResponseWriter, err error) bool {
	return CheckError(w, http.StatusInternalServerError, err)
}

// CheckError takes care of writing an error response if err is not nil.
// Returns false when err is nil; it returns true otherwise.
func CheckError(w http.ResponseWriter, status int, err error) bool {
	if err != nil {
		WriteErrorResponse(w, status, err.Error())
		return true
	}

	return false
}

// ResponseWithHeight defines a response object type that wraps an original
// response with a height.
type ResponseWithHeight struct {
	Height int64           `json:"height"`
	Result json.RawMessage `json:"result"`
}

// NewResponseWithHeight creates a new ResponseWithHeight instance
func NewResponseWithHeight(height int64, result json.RawMessage) ResponseWithHeight {
	return ResponseWithHeight{
		Height: height,
		Result: result,
	}
}
