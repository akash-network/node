package session

import (
	"fmt"
	"io"
	"os"
	"strings"
)

type textPrinter struct {
	data *PrinterData
	out  io.Writer
}

func NewTextPrinter(data *PrinterData, out io.Writer) Printer {
	if data == nil {
		data = NewPrinterDataKV()
	}
	if out == nil {
		out = os.Stdout
	}
	return &textPrinter{data: data, out: out}
}

func (t *textPrinter) Flush() error {
	switch t.data.ResultMode() {
	case PrinterResultModeKV:
		_, err := t.out.Write([]byte(t.formatKV()))
		return err
	case PrinterResultModeList:
		_, err := t.out.Write([]byte(t.formatList()))
		return err
	}
	return nil
}

func (t *textPrinter) Data() *PrinterData {
	return t.data
}

func (t *textPrinter) formatKV() string {
	if t.data == nil || len(t.data.Result) == 0 {
		return ""
	}
	var b strings.Builder
	for k, v := range t.data.Result[0] {
		fmt.Fprintf(&b, "AKASH_%s=\"%s\"\n", strings.ToUpper(k), v)
	}
	return b.String()
}

func (t *textPrinter) formatList() string {
	if t.data == nil || len(t.data.Result) == 0 {
		return ""
	}
	var b strings.Builder
	for i, res := range t.data.Result {
		for k, v := range res {
			fmt.Fprintf(&b, "AKASH_%s[%d]=\"%s\"\n", strings.ToUpper(k), i+1, v)
		}
	}
	return b.String()
}
