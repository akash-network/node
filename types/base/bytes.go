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

func (this Bytes) Compare(that Bytes) int {
	thisb, _ := this.Marshal()
	thatb, _ := that.Marshal()
	return bytes.Compare(thisb, thatb)
}
