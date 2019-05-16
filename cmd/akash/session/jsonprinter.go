package session

import (
	"encoding/json"
	"io"
	"os"
)

type jsonPrinter struct {
	data *PrinterData
	out  io.Writer
}

func (j *jsonPrinter) Flush() error {
	b, err := json.Marshal(j.data)
	if err != nil {
		return err
	}
	_, err = j.out.Write(b)
	j.data = nil
	return err
}

func (j *jsonPrinter) Data() *PrinterData {
	return j.data
}

func NewJSONPrinter(data *PrinterData, out io.Writer) Printer {
	if data == nil {
		data = &PrinterData{}
	}
	if out == nil {
		out = os.Stdout
	}
	return &jsonPrinter{out: out, data: data}
}
