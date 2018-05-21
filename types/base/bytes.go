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

func (this Bytes) Compare(that Bytes) int {
	return bytes.Compare([]byte(this), []byte(that))
}

func DecodeString(buf string) (Bytes, error) {
	val := new(Bytes)
	return *val, val.DecodeString(buf)
}
