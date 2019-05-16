package session

import (
	"time"
)

type Printer interface {
	Flush() error
	Data() *PrinterData
}

type PrinterResult map[string]string
type PrinterResultMode uint

const (
	PrinterResultModeKV PrinterResultMode = iota
	PrinterResultModeList
)

type PrinterData struct {
	Result     []map[string]string `json:"result,omitempty"`
	Raw        interface{}         `json:"raw,omitempty"`
	Log        []PrinterLog        `json:"log,omitempty"`
	resultMode PrinterResultMode
}

func (d *PrinterData) ResultMode() PrinterResultMode {
	return d.resultMode
}

func NewPrinterDataList() *PrinterData {
	return &PrinterData{resultMode: PrinterResultModeList}
}

func NewPrinterDataKV() *PrinterData {
	return &PrinterData{resultMode: PrinterResultModeKV}
}

func (d *PrinterData) AddResultKV(key, value string) *PrinterData {
	if len(d.Result) == 0 {
		d.Result = make([]map[string]string, 1, 1)
	}

	if d.Result[0] == nil {
		d.Result[0] = map[string]string{key: value}
	} else {
		d.Result[0][key] = value
	}
	return d
}

func (d *PrinterData) AddResultList(results ...PrinterResult) *PrinterData {
	if len(d.Result) == 0 {
		d.Result = make([]map[string]string, 0, 0)
	}
	for _, res := range results {
		obj := make(map[string]string)
		for k, v := range res {
			obj[k] = v
		}
		d.Result = append(d.Result, obj)
	}
	return d
}

type PrinterLogLevel string

const (
	PrinterLogLevelInfo  PrinterLogLevel = "info"
	PrinterLogLevelDebug                 = "debug"
	PrinterLogLevelError                 = "error"
)

type PrinterLog struct {
	Timestamp time.Time
	Message   interface{}
	Level     PrinterLogLevel
}
