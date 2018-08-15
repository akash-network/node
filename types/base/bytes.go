package base

import (
	"bytes"
	"encoding/hex"
	"encoding/json"

	"github.com/ovrclk/akash/util"
)

type Bytes []byte

func (t Bytes) Marshal() ([]byte, error) {
	return t, nil
}

func (t *Bytes) MarshalTo(data []byte) (n int, err error) {
	return copy(data, *t), nil
}

func (t *Bytes) Unmarshal(data []byte) error {
	*t = data
	return nil
}

func (t Bytes) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.EncodeString())
}

func (t *Bytes) UnmarshalJSON(buf []byte) error {
	var val string
	if err := json.Unmarshal(buf, &val); err != nil {
		return err
	}
	return t.DecodeString(val)
}

func (t *Bytes) DecodeString(buf string) error {
	val, err := hex.DecodeString(buf)
	*t = val
	return err
}

func (t Bytes) EncodeString() string {
	return util.X(t)
}

func (t Bytes) String() string {
	return t.EncodeString()
}

func (t Bytes) Compare(other Bytes) int {
	return bytes.Compare([]byte(t), []byte(other))
}

func (t Bytes) Equal(other Bytes) bool {
	return bytes.Equal([]byte(t), []byte(other))
}

func (t Bytes) Size() int {
	return len(t)
}

func DecodeString(buf string) (Bytes, error) {
	val := new(Bytes)
	return *val, val.DecodeString(buf)
}
