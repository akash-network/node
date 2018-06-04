package manifest

import (
	"bytes"
	"io"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/ovrclk/akash/types"
)

func unmarshalRequest(r io.Reader) (*types.ManifestRequest, error) {
	obj := &types.ManifestRequest{}
	return obj, jsonpb.Unmarshal(r, obj)
}

func marshalRequest(obj *types.ManifestRequest) ([]byte, error) {
	buf := bytes.Buffer{}
	marshaler := jsonpb.Marshaler{}
	if err := marshaler.Marshal(&buf, obj); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func unmarshal(r io.Reader) (*types.Manifest, error) {
	obj := &types.Manifest{}
	return obj, jsonpb.Unmarshal(r, obj)
}

func marshal(obj *types.Manifest) ([]byte, error) {
	buf := bytes.Buffer{}
	marshaler := jsonpb.Marshaler{}
	if err := marshaler.Marshal(&buf, obj); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
