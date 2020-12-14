package common

import (
	"bytes"
	"encoding/json"
	"github.com/cosmos/cosmos-sdk/client"
)

func PrintJSON(ctx client.Context, v interface{}) error {
	marshaled, err := json.Marshal(v)
	if err != nil {
		return err
	}

	buf := &bytes.Buffer{}
	err = json.Indent(buf, marshaled, "", "  ")
	if err != nil {
		return err
	}

	// Add a newline, for printing in the terminal
	_, err = buf.WriteRune('\n')
	if err != nil {
		return err
	}

	return ctx.PrintString(buf.String())
}
