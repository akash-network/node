package manifest

// func unmarshalRequest(r io.Reader) (*types.ManifestRequest, error) {
// 	obj := &types.ManifestRequest{}
// 	return obj, jsonpb.Unmarshal(r, obj)
// }

// func marshalRequest(obj *types.ManifestRequest) ([]byte, error) {
// 	buf := bytes.Buffer{}
// 	marshaler := jsonpb.Marshaler{}
// 	if err := marshaler.Marshal(&buf, obj); err != nil {
// 		return nil, err
// 	}
// 	return buf.Bytes(), nil
// }

// func marshal(obj proto.Message) ([]byte, error) {
// 	buf := bytes.Buffer{}
// 	marshaler := jsonpb.Marshaler{}
// 	if err := marshaler.Marshal(&buf, obj); err != nil {
// 		return nil, err
// 	}
// 	return buf.Bytes(), nil
// }
