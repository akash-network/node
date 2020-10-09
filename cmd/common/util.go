package common

import (
	"bytes"
	"encoding/json"
	"os"
)

func PrintJSONStdout(v interface{}) error {
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

	_, err = os.Stdout.Write(buf.Bytes())
	return err
}
