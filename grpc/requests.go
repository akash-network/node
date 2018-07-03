package grpc

import (
	"bytes"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
)

func Marshal(obj proto.Message) ([]byte, error) {
	buf := bytes.Buffer{}
	marshaler := jsonpb.Marshaler{}
	if err := marshaler.Marshal(&buf, obj); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
