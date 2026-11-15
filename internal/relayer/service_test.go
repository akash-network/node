package relayer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestServiceInitializationBindsGenesisWithoutPaying(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		calls.Add(1)
		if request.Method != "getGenesisHash" {
			t.Errorf("unexpected initialization RPC: %s", request.Method)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result":  mainnetGenesis,
		})
	}))
	defer server.Close()
	root := durableTestDirectory(t)
	keyPath := filepath.Join(root, "test-key.json")
	if _, err := createKey(keyPath); err != nil {
		t.Fatal(err)
	}
	values := configExample()
	values["solanaRpc"] = server.URL
	values["payerKeyFile"] = keyPath
	values["stateDir"] = filepath.Join(root, "state")
	cfg, err := parseTestConfig(t, values)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := newService(context.Background(), cfg, defaultDependencies(Streams{}))
	if err != nil {
		t.Fatal(err)
	}
	defer svc.close()
	if svc.genesis != mainnetGenesis || calls.Load() != 1 {
		t.Fatal("initialization did not bind the one observed genesis")
	}
	if _, err := os.Stat(cfg.stateDir); !os.IsNotExist(err) {
		t.Fatal("initialization mutated persistent state")
	}
	other := *cfg.genesis
	other[0] ^= 1
	cfg.genesis = &other
	if _, err := newService(context.Background(), cfg, defaultDependencies(Streams{})); err == nil {
		t.Fatal("wrong genesis accepted")
	}
}

func TestConcurrentEventsAreWholeJSONLines(t *testing.T) {
	var output bytes.Buffer
	svc := &service{
		deps: defaultDependencies(Streams{
			Out: &output,
		}),
	}
	var group sync.WaitGroup
	for i := range 20 {
		group.Add(1)
		go func() {
			defer group.Done()
			svc.emit(false, map[string]any{
				"event": "test",
				"index": i,
			})
		}()
	}
	group.Wait()
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 20 {
		t.Fatal("events were lost or merged")
	}
	for _, line := range lines {
		if !json.Valid([]byte(line)) {
			t.Fatal("concurrent events interleaved")
		}
	}
}

func TestSleepCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleep(ctx, time.Hour); err != context.Canceled {
		t.Fatalf("cancelled sleep: %v", err)
	}
}

func TestServiceIgnoresProxyEnvironment(t *testing.T) {
	// The standard transport caches proxy environment on first use. A child
	// test process lets this exercise the real default without test-order effects.
	if os.Getenv("RELAYER_SERVICE_PROXY_TEST") != "1" {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, executable, "-test.run=^TestServiceIgnoresProxyEnvironment$", "-test.v")
		command.Env = []string{"RELAYER_SERVICE_PROXY_TEST=1"}
		for _, name := range []string{"RELAYER_TEST_CACHE", "TMPDIR"} {
			if value := os.Getenv(name); value != "" {
				if !filepath.IsAbs(value) {
					t.Fatalf("%s must be an absolute test-only scratch path", name)
				}
				command.Env = append(command.Env, name+"="+value)
			}
		}
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("isolated proxy test: %v\n%s", err, output)
		}
		return
	}

	var proxyCalls, solanaCalls, nativeCalls atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalls.Add(1)
		http.Error(w, "unexpected proxy routing", http.StatusBadGateway)
	}))
	defer proxy.Close()

	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy", "ALL_PROXY", "all_proxy"} {
		t.Setenv(name, proxy.URL)
	}
	for _, name := range []string{"NO_PROXY", "no_proxy"} {
		t.Setenv(name, "")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		response := map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
		}
		switch request.Method {
		case "getGenesisHash":
			solanaCalls.Add(1)
			response["result"] = mainnetGenesis
		case "status":
			nativeCalls.Add(1)
			response["error"] = map[string]any{
				"code":    -32603,
				"message": "offline native test failure",
			}
		default:
			t.Errorf("unexpected RPC: %s", request.Method)
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	original := http.DefaultTransport
	transport := original.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		switch address {
		case "relayer-probe.invalid:3000":
			address = server.Listener.Addr().String()
		case proxy.Listener.Addr().String():
		default:
			return nil, fmt.Errorf("blocked non-probe connection: %s", address)
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	http.DefaultTransport = transport
	t.Cleanup(func() {
		http.DefaultTransport = original
		transport.CloseIdleConnections()
	})

	root := durableTestDirectory(t)
	keyPath := filepath.Join(root, "test-key.json")
	if _, err := createKey(keyPath); err != nil {
		t.Fatal(err)
	}
	values := configExample()
	values["solanaRpc"] = "http://relayer-probe.invalid:3000/solana"
	values["akashRpc"] = "http://relayer-probe.invalid:3000/akash"
	values["payerKeyFile"] = keyPath
	values["stateDir"] = filepath.Join(root, "state")
	cfg, err := parseTestConfig(t, values)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	svc, err := newService(ctx, cfg, defaultDependencies(Streams{}))
	if err != nil {
		t.Fatalf("initialization failed with %d proxy calls: %v", proxyCalls.Load(), err)
	}
	defer svc.close()

	if _, err := svc.native.Snapshot(ctx); err == nil || !strings.Contains(err.Error(), "offline native test failure") {
		t.Fatalf("native RPC must preserve its direct response error: %v", err)
	}
	if solanaCalls.Load() != 1 || nativeCalls.Load() != 1 || proxyCalls.Load() != 0 {
		t.Fatalf("routing: Solana=%d native=%d proxy=%d", solanaCalls.Load(), nativeCalls.Load(), proxyCalls.Load())
	}
}

func serviceRPCConfig(t *testing.T) config {
	t.Helper()
	root := durableTestDirectory(t)
	keyPath := filepath.Join(root, "test-key.json")
	if _, err := createKey(keyPath); err != nil {
		t.Fatal(err)
	}
	values := configExample()
	values["payerKeyFile"] = keyPath
	values["stateDir"] = filepath.Join(root, "state")
	cfg, err := parseTestConfig(t, values)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestServiceOwnedRPCRedirectPrivacy(t *testing.T) {
	cfg := serviceRPCConfig(t)
	for _, status := range []int{302, 307, 308} {
		for _, crossOrigin := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/cross-origin=%t", status, crossOrigin), func(t *testing.T) {
				bodies := make(chan []byte, 1)
				var originals, redirects atomic.Int64
				finish := func(w http.ResponseWriter, r *http.Request) {
					redirects.Add(1)
					if r.Header.Get("Referer") != "" {
						t.Error("owned client sent Referer")
					}
					if r.RequestURI != "/redirect-token-probe?own=target-token-probe" {
						t.Errorf("redirect target: %s", r.RequestURI)
					}
					original := <-bodies
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					if status == 302 {
						if r.Method != http.MethodGet || len(body) != 0 {
							t.Error("302 did not change POST to body-free GET")
						}
					} else if r.Method != http.MethodPost || !bytes.Equal(body, original) {
						t.Error("307/308 did not replay original POST body")
					}

					var input struct {
						ID     json.RawMessage `json:"id"`
						Method string          `json:"method"`
					}
					if err := json.Unmarshal(original, &input); err != nil {
						t.Error(err)
						return
					}
					response := map[string]any{
						"jsonrpc": "2.0",
						"id":      input.ID,
					}
					if input.Method == "getGenesisHash" {
						response["result"] = mainnetGenesis
					} else if input.Method == "status" {
						response["error"] = map[string]any{
							"code":    -32603,
							"message": "inert native fixture",
						}
					} else {
						t.Errorf("unexpected RPC: %s", input.Method)
					}
					if err := json.NewEncoder(w).Encode(response); err != nil {
						t.Error(err)
					}
				}
				destination := httptest.NewServer(http.HandlerFunc(finish))
				defer destination.Close()
				origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/redirect-token-probe" {
						finish(w, r)
						return
					}
					originals.Add(1)
					if r.RequestURI != "/path-token-probe?token=query-token-probe&key=%2f&key=2" {
						t.Errorf("original target: %s", r.RequestURI)
					}
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					bodies <- body
					location := "/redirect-token-probe?own=target-token-probe"
					if crossOrigin {
						location = destination.URL + location
					}
					w.Header().Set("Location", location)
					w.WriteHeader(status)
				}))
				defer origin.Close()

				cfg.solanaRPC = origin.URL + "/path-token-probe?token=query-token-probe&key=%2f&key=2"
				cfg.akashRPC = cfg.solanaRPC
				svc, err := newService(context.Background(), cfg, defaultDependencies(Streams{}))
				if err != nil {
					t.Fatal(err)
				}
				defer svc.close()
				_, err = svc.native.Snapshot(context.Background())
				if err == nil || !strings.Contains(err.Error(), "inert native fixture") {
					t.Fatalf("native response: %v", err)
				}
				if originals.Load() != 2 || redirects.Load() != 2 {
					t.Fatal("both owned clients must follow exactly one redirect")
				}
			})
		}
	}
}

func TestServiceSolanaRedirectLimitAndErrorEvents(t *testing.T) {
	cfg := serviceRPCConfig(t)
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Referer") != "" {
			t.Error("owned Solana client sent Referer")
		}
		w.Header().Set("Location", "/redirect-token-probe?token=redirect-query-probe")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	cfg.solanaRPC = server.URL + "/path-token-probe?token=query-token-probe"
	_, err := newService(context.Background(), cfg, defaultDependencies(Streams{}))
	if err == nil || !strings.Contains(err.Error(), "stopped after 10 redirects") {
		t.Fatalf("redirect failure: %v", err)
	}
	if calls.Load() != 10 {
		t.Fatalf("redirect limit changed: got %d requests", calls.Load())
	}

	var output bytes.Buffer
	svc := &service{
		deps: defaultDependencies(Streams{
			Err: &output,
		}),
	}
	svc.emit(true, map[string]any{
		"event": "rpc-failure",
		"error": fmt.Errorf("service initialization: %w", err).Error(),
	})
	if strings.Contains(output.String(), "probe") {
		t.Fatal("RPC endpoint credential appeared in emitted error")
	}
	if !strings.Contains(output.String(), "stopped after 10 redirects") {
		t.Fatal("RPC failure diagnostic lost from event")
	}
}
