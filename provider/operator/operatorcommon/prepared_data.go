package operatorcommon

import (
	"net/http"
	"sync/atomic"
	"time"
)

const (
	preparedResultByteSizeLimit = 8388608 // 8 megabtyes
)

type PreparedResult interface {
	Flag()
	Set([]byte)
}

type preparedResultData struct {
	preparedAt time.Time
	data       []byte
}

type preparedResult struct {
	needsPrepare bool
	data         atomic.Value
}

func newPreparedResult() *preparedResult {
	result := &preparedResult{
		needsPrepare: true,
	}
	result.Set([]byte{})
	return result
}

func (pr *preparedResult) Flag() {
	pr.needsPrepare = true
}

func (pr *preparedResult) Set(data []byte) {
	// Limit the length of the value
	// This is done because storing a precomputed result is entirely sensible (it scaled well)
	// but this code path should never allow us to store arbitrarily large serialized results
	// which can happen as the result of other programming errors. If this happens just truncate the
	// result so some debugging is still possible
	if len(data) > preparedResultByteSizeLimit {
		data = data[0:preparedResultByteSizeLimit]
	}
	pr.needsPrepare = false
	pr.data.Store(preparedResultData{
		preparedAt: time.Now(),
		data:       data,
	})
}

func (pr *preparedResult) get() preparedResultData {
	return (pr.data.Load()).(preparedResultData)
}

func servePreparedResult(rw http.ResponseWriter, pd *preparedResult) {
	rw.Header().Set("Cache-Control", "no-cache, max-age=0")
	value := pd.get()
	if len(value.data) == 0 {
		rw.WriteHeader(http.StatusNoContent)
		return
	}

	rw.Header().Set("Last-Modified", value.preparedAt.UTC().Format(http.TimeFormat))
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write(value.data)
}
