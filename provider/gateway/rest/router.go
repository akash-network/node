package rest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/ovrclk/akash/util/wsutil"
	"io"
	"k8s.io/client-go/tools/remotecommand"
	"net/http"
	"sync"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	gcontext "github.com/gorilla/context"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"github.com/tendermint/tendermint/libs/log"

	"github.com/gorilla/mux"

	kubeErrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/provider"
	"github.com/ovrclk/akash/provider/cluster"
	kubeClient "github.com/ovrclk/akash/provider/cluster/kube"
	cltypes "github.com/ovrclk/akash/provider/cluster/types"
	pmanifest "github.com/ovrclk/akash/provider/manifest"
	manifestValidation "github.com/ovrclk/akash/validation"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

type CtxAuthKey string

const (
	contentTypeJSON = "application/json; charset=UTF-8"

	// Time allowed to write the file to the client.
	pingWait = 15 * time.Second

	// Time allowed to read the next pong message from the client.
	pongWait = 15 * time.Second

	// Send pings to client with this period. Must be less than pongWait.
	pingPeriod = 10 * time.Second
)

const (
	// as per RFC https://www.iana.org/assignments/websocket/websocket.xhtml#close-code-number
	// errors from private use staring
	websocketInternalServerErrorCode = 4000
	websocketLeaseNotFound           = 4001
)

type wsStreamConfig struct {
	lid       mtypes.LeaseID
	services  string
	follow    bool
	tailLines *int64
	log       log.Logger
	client    cluster.ReadClient
}

func newRouter(log log.Logger, addr sdk.Address, pclient provider.Client) *mux.Router {
	router := mux.NewRouter()

	// store provider address in context as lease endpoints below need it
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gcontext.Set(r, providerContextKey, addr)

			next.ServeHTTP(w, r)
		})
	})

	// GET /status
	// provider status endpoint does not require authentication
	router.HandleFunc("/status",
		createStatusHandler(log, pclient)).
		Methods("GET")

	// GET /validate
	// validate endpoint checks if provider will bid on given groupspec
	router.HandleFunc("/validate",
		validateHandler(log, pclient)).
		Methods("GET")

	// PUT /deployment/manifest
	drouter := router.PathPrefix(deploymentPathPrefix).Subrouter()
	drouter.Use(
		requireOwner(),
		requireDeploymentID(),
	)

	drouter.HandleFunc("/manifest",
		createManifestHandler(log, pclient.Manifest())).
		Methods("PUT")

	lrouter := router.PathPrefix(leasePathPrefix).Subrouter()
	lrouter.Use(
		requireOwner(),
		requireLeaseID(),
	)

	// GET /lease/<lease-id>/status
	lrouter.HandleFunc("/status",
		leaseStatusHandler(log, pclient.Cluster())).
		Methods("GET")

	// GET /lease/<lease-id>/kubeevents
	eventsRouter := lrouter.PathPrefix("/kubeevents").Subrouter()
	eventsRouter.Use(
		requestStreamParams(),
	)
	eventsRouter.HandleFunc("",
		leaseKubeEventsHandler(log, pclient.Cluster())).
		Methods("GET")

	logRouter := lrouter.PathPrefix("/logs").Subrouter()
	logRouter.Use(
		requestStreamParams(),
	)

	// GET /lease/<lease-id>/logs
	logRouter.HandleFunc("",
		leaseLogsHandler(log, pclient.Cluster())).
		Methods("GET")

	srouter := lrouter.PathPrefix("/service/{serviceName}").Subrouter()
	srouter.Use(
		requireService(),
	)

	// GET /lease/<lease-id>/service/<service-name>/status
	srouter.HandleFunc("/status",
		leaseServiceStatusHandler(log, pclient.Cluster())).
		Methods("GET")

	// POST /lease/<lease-id>/shell
	lrouter.HandleFunc("/shell",
		leaseShellHandler(log, pclient.Cluster()))

	return router
}

type channelToTerminalSizeQueue <- chan remotecommand.TerminalSize

func (sq channelToTerminalSizeQueue) Next() *remotecommand.TerminalSize {
	v, ok := <- sq
	if !ok {
		return nil
	}

	return &v // Interface is dumb and use a pointer
}

type leaseShellResponse struct {
	ExitCode int `json:"exit_code"`
	Message string `json:"message,omitempty"`
}

func leaseShellHandler(log log.Logger, cclient cluster.Client) http.HandlerFunc {
	return func (rw http.ResponseWriter, req *http.Request){

		vars := req.URL.Query()
		var cmd []string

		for i := 0; true; i++ {
			v := vars.Get(fmt.Sprintf("cmd%d", i))
			if 0 == len(v) {
				break
			}
			cmd = append(cmd, v)
		}

		if len(cmd) == 0{
			log.Error("missing cmd parameter")
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
		tty := vars.Get("tty")
		if 0 == len(tty) {
			log.Error("missing parameter tty")
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
		isTty := tty == "1"

		service := vars.Get("service")
		if 0 == len(service) {
			log.Error("missing parameter service")
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}

		stdin := vars.Get("stdin")
		if 0 == len(stdin) {
			log.Error("missing parameter stdin")
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
		connectStdin := stdin == "1"

		upgrader := websocket.Upgrader{
			ReadBufferSize:  0,
			WriteBufferSize: 0,
		}

		ws, err := upgrader.Upgrade(rw, req, nil)
		if err != nil {
			// At this point the connection either has a response sent already
			// or it has been closed
			log.Error("failed handshake", "err", err)
			return
		}

		var stdinPipeOut *io.PipeWriter
		var stdinPipeIn *io.PipeReader
		wg := &sync.WaitGroup{}

		var tsq remotecommand.TerminalSizeQueue
		var terminalSizeUpdate chan remotecommand.TerminalSize
		if isTty {
			terminalSizeUpdate = make(chan remotecommand.TerminalSize, 1)
			tsq = channelToTerminalSizeQueue(terminalSizeUpdate)
		}

		if connectStdin {
			stdinPipeIn, stdinPipeOut = io.Pipe()

			wg.Add(1)
			go func () {
				defer wg.Done()
				for {
					msgType, data, err := ws.ReadMessage()
					if err != nil {
						return
					}

					if msgType == websocket.BinaryMessage && len(data) > 1  {
						msgId := data[0]
						msg := data[1:]
						if msgId == LeaseShellCodeStdin {
							_, err := stdinPipeOut.Write(msg)
							if err != nil {
								return
							}
						}
						if msgId == LeaseShellCodeTerminalResize {
							var size remotecommand.TerminalSize
							r := bytes.NewReader(msg)
							err = binary.Read(r, binary.BigEndian, &size.Width)
							if err != nil {
								return
							}
							err = binary.Read(r, binary.BigEndian, &size.Height)
							if err != nil {
								return
							}

							log.Debug("terminal resize received", "width", size.Width, "height", size.Height)
							if terminalSizeUpdate != nil {
								terminalSizeUpdate <- size
							}
						}
					}
				}

			}()
		}

		l := &sync.Mutex{}
		stdout := wsutil.NewWsWriterWrapper(ws, LeaseShellCodeStdout, l)
		stderr := wsutil.NewWsWriterWrapper(ws, LeaseShellCodeStderr, l)

		var stdinForExec io.Reader
		if connectStdin {
			stdinForExec = stdinPipeIn
		}
		result, err := cclient.Exec(req.Context(), requestLeaseID(req), service, cmd, stdinForExec, stdout, stderr, isTty, tsq)

		responseData := leaseShellResponse{}

		var resultWriter io.Writer

		encodeData := true

		resultWriter = wsutil.NewWsWriterWrapper(ws, LeaseShellCodeResult, l)

		if result != nil {
			responseData.ExitCode = result.ExitCode()

			log.Debug("lease shell completed", "exitcode", result.ExitCode())
		} else {
			if cluster.ErrorIsOkToSendToClient(err) {
				responseData.Message = err.Error()
			} else {
				resultWriter = wsutil.NewWsWriterWrapper(ws, LeaseShellCodeFailure, l)
				// Don't return errors like this to the client, they could contain information
				// that should not be let out
				encodeData = false

				log.Error("lease exec failed", "err", err)
			}
		}

		if encodeData {
			encoder := json.NewEncoder(resultWriter)
			err = encoder.Encode(responseData)
		} else {
			// Just send an empty message so the remote knows things are over
			_, err = resultWriter.Write([]byte{})
		}

		_ = ws.Close()

		if err != nil {
			log.Error("failed writing response to client after exec", "err", err)
		}

		wg.Wait()

		if stdinPipeOut != nil {
			_ = stdinPipeOut.Close()
		}
		if stdinPipeIn != nil {
			_ = stdinPipeIn.Close()
		}

		if terminalSizeUpdate != nil {
			close(terminalSizeUpdate)
		}

		return
	}
}

func createStatusHandler(log log.Logger, sclient provider.StatusClient) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		status, err := sclient.Status(req.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(log, w, status)
	}
}

func validateHandler(log log.Logger, cl provider.ValidateClient) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		data, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if len(data) == 0 {
			http.Error(w, "empty payload", http.StatusBadRequest)
			return
		}

		var gspec dtypes.GroupSpec

		if err := json.Unmarshal(data, &gspec); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}

		validate, err := cl.Validate(req.Context(), gspec)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(log, w, validate)
	}
}

func createManifestHandler(_ log.Logger, mclient pmanifest.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var mani manifest.Manifest
		decoder := json.NewDecoder(req.Body)
		defer func() {
			_ = req.Body.Close()
		}()

		if err := decoder.Decode(&mani); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}

		if err := mclient.Submit(req.Context(), requestDeploymentID(req), mani); err != nil {
			if errors.Is(err, manifestValidation.ErrInvalidManifest) {
				http.Error(w, err.Error(), http.StatusUnprocessableEntity)
				return
			}
			if errors.Is(err, pmanifest.ErrNoLeaseForDeployment) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func leaseKubeEventsHandler(log log.Logger, cclient cluster.ReadClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		}

		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			// At this point the connection either has a response sent already
			// or it has been closed
			return
		}

		wsEventWriter(r.Context(), ws, wsStreamConfig{
			lid:      requestLeaseID(r),
			follow:   requestLogFollow(r),
			services: requestServices(r),
			log:      log,
			client:   cclient,
		})
	}
}

func leaseStatusHandler(log log.Logger, cclient cluster.ReadClient) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		status, err := cclient.LeaseStatus(req.Context(), requestLeaseID(req))
		if err != nil {
			if errors.Is(err, kubeClient.ErrNoDeploymentForLease) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			if errors.Is(err, kubeClient.ErrLeaseNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			if kubeErrors.IsNotFound(err) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			if errors.Is(err, kubeClient.ErrNoGlobalServicesForLease) {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(log, w, status)
	}
}

func leaseServiceStatusHandler(log log.Logger, cclient cluster.ReadClient) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		status, err := cclient.ServiceStatus(req.Context(), requestLeaseID(req), requestService(req))
		if err != nil {
			if errors.Is(err, kubeClient.ErrNoDeploymentForLease) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			if errors.Is(err, kubeClient.ErrLeaseNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			if kubeErrors.IsNotFound(err) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(log, w, status)
	}
}

func leaseLogsHandler(log log.Logger, cclient cluster.ReadClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		}

		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			// At this point the connection either has a response sent already
			// or it has been closed
			return
		}

		wsLogWriter(r.Context(), ws, wsStreamConfig{
			lid:       requestLeaseID(r),
			services:  requestServices(r),
			follow:    requestLogFollow(r),
			tailLines: requestLogTailLines(r),
			log:       log,
			client:    cclient,
		})
	}
}

func wsSetupPongHandler(ws *websocket.Conn, cancel func()) error {
	if err := ws.SetReadDeadline(time.Time{}); err != nil {
		return err
	}

	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(pingWait))
	})

	go func() {
		var err error

		defer func() {
			if err != nil {
				cancel()
			}
		}()

		for {
			var mtype int
			if mtype, _, err = ws.ReadMessage(); err != nil {
				break
			}

			if mtype == websocket.CloseMessage {
				err = errors.Errorf("disconnect")
			}
		}
	}()

	return nil
}

func wsLogWriter(ctx context.Context, ws *websocket.Conn, cfg wsStreamConfig) {
	pingTicker := time.NewTicker(pingPeriod)

	cctx, cancel := context.WithCancel(ctx)
	defer func() {
		pingTicker.Stop()
		cancel()
		_ = ws.Close()
	}()

	logs, err := cfg.client.LeaseLogs(cctx, cfg.lid, cfg.services, cfg.follow, cfg.tailLines)
	if err != nil {
		cfg.log.Error("couldn't fetch logs", "error", err.Error())
		err = ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocketInternalServerErrorCode, ""))
		if err != nil {
			cfg.log.Error("couldn't push control message through websocket: %s", err.Error())
		}
		return
	}

	if len(logs) == 0 {
		_ = ws.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocketInternalServerErrorCode, "no running pods"))
		return
	}

	if err = wsSetupPongHandler(ws, cancel); err != nil {
		return
	}

	var scanners sync.WaitGroup

	logch := make(chan ServiceLogMessage)

	scanners.Add(len(logs))

	for _, lg := range logs {
		go func(name string, scan *bufio.Scanner) {
			defer scanners.Done()

			for scan.Scan() && ctx.Err() == nil {
				logch <- ServiceLogMessage{
					Name:    name,
					Message: scan.Text(),
				}
			}
		}(lg.Name, lg.Scanner)
	}

	donech := make(chan struct{})

	go func() {
		scanners.Wait()
		close(donech)
	}()

done:
	for {
		select {
		case line := <-logch:
			if err = ws.WriteJSON(line); err != nil {
				break done
			}
		case <-pingTicker.C:
			if err = ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				break done
			}
			if err = ws.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
				break done
			}
		case <-donech:
			break done
		}
	}

	cancel()

	for i := range logs {
		_ = logs[i].Stream.Close()
	}

	// drain logs channel in separate goroutine to unblock seeders waiting for write space
	go func() {
	drain:
		for {
			select {
			case <-donech:
				break drain
			case <-logch:
			}
		}
	}()
}

func wsEventWriter(ctx context.Context, ws *websocket.Conn, cfg wsStreamConfig) {
	pingTicker := time.NewTicker(pingPeriod)
	cctx, cancel := context.WithCancel(ctx)
	defer func() {
		pingTicker.Stop()
		cancel()
		_ = ws.Close()
	}()

	evts, err := cfg.client.LeaseEvents(cctx, cfg.lid, cfg.services, cfg.follow)
	if err != nil {
		cfg.log.Error("couldn't fetch events", "error", err.Error())
		err = ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocketInternalServerErrorCode, ""))
		if err != nil {
			cfg.log.Error("couldn't push control message through websocket", "error", err.Error())
		}
		return
	}

	if evts == nil {
		err = ws.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocketLeaseNotFound, ""))
		if err != nil {
			cfg.log.Error("couldn't push control message through websocket", "error", err.Error())
		}
		return
	}

	defer evts.Shutdown()

	if err = wsSetupPongHandler(ws, cancel); err != nil {
		return
	}

	sendClose := func() {
		_ = ws.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}
done:
	for {
		select {
		case <-ctx.Done():
			sendClose()
			break done
		case <-evts.Done():
			sendClose()
			break done
		case evt := <-evts.ResultChan():
			if evt == nil {
				sendClose()
				break done
			}

			if err = ws.WriteJSON(cltypes.LeaseEvent{
				Type:                evt.Type,
				ReportingController: evt.ReportingController,
				ReportingInstance:   evt.ReportingInstance,
				Time:                evt.EventTime.Time,
				Reason:              evt.Reason,
				Note:                evt.Note,
				Object: cltypes.LeaseEventObject{
					Kind:      evt.Regarding.Kind,
					Namespace: evt.Regarding.Namespace,
					Name:      evt.Regarding.Name,
				},
			}); err != nil {
				break done
			}
		case <-pingTicker.C:
			if err = ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				break done
			}
			if err = ws.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
				break done
			}
		}
	}
}

func writeJSON(log log.Logger, w http.ResponseWriter, obj interface{}) {
	bytes, err := json.Marshal(obj)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", contentTypeJSON)

	_, err = w.Write(bytes)
	if err != nil {
		log.Error("error writing response", "err", err)
		return
	}
}
