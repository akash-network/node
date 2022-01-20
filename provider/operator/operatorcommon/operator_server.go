package operatorcommon

import (
	"bytes"
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/ovrclk/akash/provider/gateway/utils"
	"io"
	"net/http"
)

type PrepareFlagFn func()
type PrepareFn func(pd PreparedResult) error
type preparedEntry struct {
	data    *preparedResult
	prepare PrepareFn
}

type OperatorHTTP interface {
	AddPreparedEndpoint(path string, prepare PrepareFn) PrepareFlagFn
	GetRouter() *mux.Router
	PrepareAll() error
}

type operatorHTTP struct {
	router  *mux.Router
	results map[string]preparedEntry
}

func NewOperatorHTTP() (OperatorHTTP, error) {
	retval := &operatorHTTP{
		router:  mux.NewRouter(),
		results: make(map[string]preparedEntry),
	}

	retval.router.HandleFunc("/health", func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(rw, "OK")
	})

	akashVersion := utils.NewAkashVersionInfo()
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	err := enc.Encode(akashVersion)

	if err != nil {
		return nil, err
	}

	akashVersionJSON := buf.Bytes()
	buf = nil // remove from scope
	enc = nil // remove from scope

	retval.router.HandleFunc("/version", func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = io.Copy(rw, bytes.NewReader(akashVersionJSON))
	}).Methods("GET")

	return retval, nil
}

func (opHttp *operatorHTTP) GetRouter() *mux.Router {
	return opHttp.router
}

func (opHttp *operatorHTTP) AddPreparedEndpoint(path string, prepare PrepareFn) PrepareFlagFn {
	_, exists := opHttp.results[path]
	if exists {
		panic("prepared result exists for path: " + path)
	}

	entry := preparedEntry{
		data:    newPreparedResult(),
		prepare: prepare,
	}
	opHttp.results[path] = entry

	opHttp.router.HandleFunc(path, func(rw http.ResponseWriter, req *http.Request) {
		servePreparedResult(rw, entry.data)
	}).Methods(http.MethodGet)

	return entry.data.Flag
}

func (opHttp *operatorHTTP) PrepareAll() error {
	for _, entry := range opHttp.results {
		if !entry.data.needsPrepare {
			continue
		}
		err := entry.prepare(entry.data)
		if err != nil {
			return err
		}
	}

	return nil
}
