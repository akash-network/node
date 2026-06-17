package grpcsuite

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"

	refv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"
	refv1alpha "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

// inScopePrefixes are the proto package prefixes whose query surface the coverage
// gate enforces. Akash + wasm transactions are additionally gated (see Discovery).
var inScopePrefixes = []string{
	"akash.",
	"cosmwasm.wasm.",
	"cosmos.bank.",
	"cosmos.staking.",
	"cosmos.gov.",
	"cosmos.distribution.",
	"cosmos.authz.",
	"cosmos.feegrant.",
	"cosmos.slashing.",
	"cosmos.mint.",
	"cosmos.auth.",
	"cosmos.upgrade.",
	"cosmos.params.",
	"cosmos.consensus.",
	"cosmos.evidence.",
}

// strictMsgPrefixes are packages whose every served Msg RPC MUST be exercised by an
// authored tx case. This is the Akash-specific API surface that an upgrade can
// regress and that nothing upstream covers. Cosmos/wasm txs are exercised
// opportunistically but not gated.
var strictMsgPrefixes = []string{"akash."}

func inScope(name string) bool {
	for _, p := range inScopePrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func strictMsg(name string) bool {
	for _, p := range strictMsgPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// Discovery is the set of gRPC services the running binary actually serves,
// resolved to concrete Msg/Query methods. It is built from gRPC server reflection
// (which service names are live) intersected with the locally linked descriptors
// (which give the method lists), so it tracks the branch under test: e.g.
// akash.verification.* appears on AEP-86 but not on main.
type Discovery struct {
	// served is the set of service full names returned by reflection.
	served map[string]bool
	// msgs maps an in-scope Msg request type URL -> owning service full name.
	msgs map[string]string
	// queries maps an in-scope query method path -> owning service full name.
	queries map[string]string
}

// ServesService reports whether the running binary serves the named gRPC service.
func (d *Discovery) ServesService(fullName string) bool { return d.served[fullName] }

// HasModule reports whether the module identified by its proto package (e.g.
// "akash.deployment.v1beta4") is live, detected by its served Query service. Packs
// use this to skip modules absent on the branch under test. Note: Msg services are
// not served over gRPC, so a module's presence is keyed on its Query service.
func (d *Discovery) HasModule(pkg string) bool { return d.served[pkg+".Query"] }

// InScopeMsgs returns the in-scope Msg request type URLs that are gated.
func (d *Discovery) InScopeMsgs() []string { return keys(d.msgs) }

// InScopeQueries returns the in-scope query method paths that are gated.
func (d *Discovery) InScopeQueries() []string { return keys(d.queries) }

// discover lists served services via reflection, then resolves each in-scope
// service's methods from the linked descriptor set.
func discover(ctx context.Context, conn *grpc.ClientConn, ireg codectypes.InterfaceRegistry) (*Discovery, error) {
	served, err := listServices(ctx, conn)
	if err != nil {
		return nil, err
	}

	// Index every service descriptor linked into this binary by full name.
	svcByName := map[string]protoreflect.ServiceDescriptor{}
	ireg.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		svcs := fd.Services()
		for i := 0; i < svcs.Len(); i++ {
			sd := svcs.Get(i)
			svcByName[string(sd.FullName())] = sd
		}
		return true
	})

	d := &Discovery{
		served:  map[string]bool{},
		msgs:    map[string]string{},
		queries: map[string]string{},
	}
	for _, name := range served {
		d.served[name] = true
	}

	// Query services ARE served over gRPC, so reflection + descriptors give the
	// exact set of live query methods. The set of served Query packages also tells
	// us each module's ACTIVE version (e.g. akash.deployment.v1beta4), which we use
	// below to keep inactive-version messages out of the gated Msg set.
	servedQueryPkgs := map[string]bool{}
	for name := range d.served {
		if strings.HasSuffix(name, ".Query") {
			servedQueryPkgs[strings.TrimSuffix(name, ".Query")] = true
		}
	}
	for name := range d.served {
		if !inScope(name) || !strings.HasSuffix(name, ".Query") {
			continue
		}
		sd, ok := svcByName[name]
		if !ok {
			continue
		}
		methods := sd.Methods()
		for i := 0; i < methods.Len(); i++ {
			d.queries["/"+name+"/"+string(methods.Get(i).Name())] = name
		}
	}

	// Msg services are NOT served as callable gRPC services (messages are routed
	// through the tx service / MsgServiceRouter), so reflection does not list them.
	// Instead, enumerate every registered Msg implementation and keep the in-scope
	// ones whose package corresponds to a live Query service (the active version).
	for _, url := range ireg.ListImplementations(sdkMsgInterfaceName) {
		u := withSlash(url)
		trimmed := strings.TrimPrefix(u, "/")
		if !strictMsg(trimmed) {
			continue
		}
		idx := strings.LastIndex(trimmed, ".")
		if idx < 0 {
			continue
		}
		if pkg := trimmed[:idx]; servedQueryPkgs[pkg] {
			d.msgs[u] = pkg
		}
	}
	return d, nil
}

// sdkMsgInterfaceName is the interface registry name under which all sdk.Msg
// implementations are registered.
const sdkMsgInterfaceName = "cosmos.base.v1beta1.Msg"

func withSlash(s string) string {
	if strings.HasPrefix(s, "/") {
		return s
	}
	return "/" + s
}

// listServices returns the full names of every gRPC service the server exposes,
// trying reflection v1 first and falling back to v1alpha.
func listServices(ctx context.Context, conn *grpc.ClientConn) ([]string, error) {
	if names, err := listServicesV1(ctx, conn); err == nil {
		return names, nil
	}
	return listServicesV1Alpha(ctx, conn)
}

func listServicesV1(ctx context.Context, conn *grpc.ClientConn) ([]string, error) {
	cl := refv1.NewServerReflectionClient(conn)
	stream, err := cl.ServerReflectionInfo(ctx)
	if err != nil {
		return nil, err
	}
	if err := stream.Send(&refv1.ServerReflectionRequest{
		MessageRequest: &refv1.ServerReflectionRequest_ListServices{ListServices: "*"},
	}); err != nil {
		return nil, err
	}
	resp, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	ls := resp.GetListServicesResponse()
	if ls == nil {
		return nil, fmt.Errorf("reflection v1: unexpected response %T", resp.MessageResponse)
	}
	out := make([]string, 0, len(ls.Service))
	for _, s := range ls.Service {
		out = append(out, s.Name)
	}
	return out, nil
}

func listServicesV1Alpha(ctx context.Context, conn *grpc.ClientConn) ([]string, error) {
	cl := refv1alpha.NewServerReflectionClient(conn)
	stream, err := cl.ServerReflectionInfo(ctx)
	if err != nil {
		return nil, err
	}
	if err := stream.Send(&refv1alpha.ServerReflectionRequest{
		MessageRequest: &refv1alpha.ServerReflectionRequest_ListServices{ListServices: "*"},
	}); err != nil {
		return nil, err
	}
	resp, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	ls := resp.GetListServicesResponse()
	if ls == nil {
		return nil, fmt.Errorf("reflection v1alpha: unexpected response %T", resp.MessageResponse)
	}
	out := make([]string, 0, len(ls.Service))
	for _, s := range ls.Service {
		out = append(out, s.Name)
	}
	return out, nil
}

// Coverage tracks which Msg type URLs and query method paths were actually
// exercised, and asserts the in-scope expected set was fully covered.
type Coverage struct {
	mu sync.Mutex

	strict bool

	recordedMethods map[string]bool // gRPC method paths invoked (queries + tx service)
	recordedMsgs    map[string]bool // Msg request type URLs broadcast

	expectedMsgs    map[string]bool
	expectedQueries map[string]bool
}

func newCoverage() *Coverage {
	return &Coverage{
		recordedMethods: map[string]bool{},
		recordedMsgs:    map[string]bool{},
		expectedMsgs:    map[string]bool{},
		expectedQueries: map[string]bool{},
	}
}

func (c *Coverage) setStrict(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.strict = v
}

func (c *Coverage) recordMethod(method string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recordedMethods[method] = true
}

// recordMsg notes that a message of the given type URL was broadcast.
func (c *Coverage) recordMsg(typeURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !strings.HasPrefix(typeURL, "/") {
		typeURL = "/" + typeURL
	}
	c.recordedMsgs[typeURL] = true
}

func (c *Coverage) setExpected(d *Discovery) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for m := range d.msgs {
		c.expectedMsgs[m] = true
	}
	for q := range d.queries {
		c.expectedQueries[q] = true
	}
}

// assert reports the coverage summary and, when strict, fails the test if any
// in-scope Msg or query RPC was never exercised.
func (c *Coverage) assert(t *testing.T) {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()

	var missingMsgs, missingQueries []string
	for m := range c.expectedMsgs {
		if !c.recordedMsgs[m] {
			missingMsgs = append(missingMsgs, m)
		}
	}
	for q := range c.expectedQueries {
		if !c.recordedMethods[q] {
			missingQueries = append(missingQueries, q)
		}
	}
	sort.Strings(missingMsgs)
	sort.Strings(missingQueries)

	t.Logf("coverage: tx %d/%d, query %d/%d",
		len(c.expectedMsgs)-len(missingMsgs), len(c.expectedMsgs),
		len(c.expectedQueries)-len(missingQueries), len(c.expectedQueries))

	for _, m := range missingMsgs {
		t.Logf("coverage: UNCOVERED tx %s", m)
	}
	for _, q := range missingQueries {
		t.Logf("coverage: UNCOVERED query %s", q)
	}

	if c.strict && (len(missingMsgs) > 0 || len(missingQueries) > 0) {
		t.Errorf("coverage gate: %d tx and %d query RPC(s) were never exercised (see UNCOVERED logs above)",
			len(missingMsgs), len(missingQueries))
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
