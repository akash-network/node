package manifest

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type Error struct {
	Message string
}

func RunServ(port, loglevel string) {
	run(port, loglevel)
}

func logRequest(r *http.Request) {
	log.WithFields(log.Fields{
		"route": r.URL.Path,
	}).Debug("log request")
}

func returnError(w http.ResponseWriter, status int, message string) {
	err := Error{message}
	w.WriteHeader(status)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(err)
}

// user enters password and agrees to terms
func requestHandler(w http.ResponseWriter, r *http.Request) {
	logRequest(r)
	// get json body
	if r.Body == nil {
		returnError(w, http.StatusBadRequest, "Empty request body")
		return
	}

	// unmarshal body
	body := &Body{}
	err := json.NewDecoder(r.Body).Decode(body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Error decoding body")
		return
	}
	log.Debug(fmt.Sprintf("%+v", body))

	// XXX check if signer is tenant of lease

	// respond with success
	w.WriteHeader(http.StatusOK)
}

func run(port, loglevel string) {
	level, err := log.ParseLevel(loglevel)
	if err != nil {
		panic(err)
	}
	log.SetLevel(level)

	r := mux.NewRouter()

	// routes
	r.HandleFunc("/manifest", requestHandler)

	log.Info("Started manifest server")
	http.ListenAndServe(":"+port, r)
}
