package grpcsuite

import (
	"sort"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/status"
)

// rawCodec is a gRPC codec that passes message bodies through as raw bytes. It lets
// the smoke sweep invoke any query method with an empty request and ignore the
// response body, without needing each method's concrete request/response Go types.
type rawCodec struct{}

func (rawCodec) Marshal(v interface{}) ([]byte, error) {
	switch b := v.(type) {
	case []byte:
		return b, nil
	case *[]byte:
		return *b, nil
	default:
		return nil, status.Errorf(codes.Internal, "rawCodec.Marshal: unexpected %T", v)
	}
}

func (rawCodec) Unmarshal(data []byte, v interface{}) error {
	if p, ok := v.(*[]byte); ok {
		*p = data
		return nil
	}
	return status.Errorf(codes.Internal, "rawCodec.Unmarshal: unexpected %T", v)
}

func (rawCodec) Name() string { return "grpcsuite-raw-bytes" }

func init() { encoding.RegisterCodec(rawCodec{}) }

// querySmokeSweep invokes every discovered in-scope query method with an empty
// request over gRPC. This is the dynamic half of coverage: it reaches every query
// handler (recording it via the connection's coverage interceptor) and fails only
// when a method advertised by reflection is actually Unimplemented — a real gRPC
// wiring regression. Business errors (e.g. NotFound/InvalidArgument from an empty
// request) are expected and prove the handler is reachable; the authored query
// cases verify correctness with real inputs.
func (s *Suite) querySmokeSweep(disc *Discovery) {
	s.T.Helper()

	methods := make([]string, 0, len(disc.queries))
	for m := range disc.queries {
		methods = append(methods, m)
	}
	sort.Strings(methods)

	var unimplemented int
	for _, method := range methods {
		var reply []byte
		err := s.Conn.Invoke(s.Ctx, method, []byte{}, &reply, grpc.ForceCodec(rawCodec{}))
		if status.Code(err) == codes.Unimplemented {
			unimplemented++
			s.T.Errorf("query smoke sweep: %s is advertised by reflection but Unimplemented", method)
		}
	}
	s.logf("query smoke sweep: invoked %d query methods (%d unimplemented)", len(methods), unimplemented)
}
