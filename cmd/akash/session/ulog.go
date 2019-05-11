package session

import (
	"fmt"
	"io"
	"os"

	"github.com/ovrclk/akash/util/ulog"
)

type ULog interface {
	Error(msg interface{})
	Success(msg interface{})
}

func NewUlogger(s Session) ULog {
	return &ulogger{
		s:      s,
		out:    os.Stdout,
		errOut: os.Stderr,
	}
}

type ulogger struct {
	s      Session
	out    io.Writer
	errOut io.Writer
}

func (u *ulogger) Error(msg interface{}) {
	printerDat := NewPrinterDataKV().AddResultKV("error", fmt.Sprintf("%v", msg))
	u.s.Mode().
		When(ModeTypeInteractive, func() error {
			fmt.Fprintln(u.errOut, "")
			fmt.Fprintln(u.errOut, ulog.Error(fmt.Sprintf("%v", msg)))
			return nil
		}).
		When(ModeTypeText, func() error {
			NewTextPrinter(printerDat, nil).Flush()
			return nil
		}).
		When(ModeTypeJSON, func() error {
			NewJSONPrinter(printerDat, nil).Flush()
			return nil
		}).Run()
}

func (u *ulogger) Success(msg interface{}) {
	printerDat := NewPrinterDataKV().AddResultKV("success", fmt.Sprintf("%v", msg))
	u.s.Mode().
		When(ModeTypeInteractive, func() error {
			fmt.Fprintln(u.out, "")
			fmt.Fprintln(u.out, ulog.Success(fmt.Sprintf("%v", msg)))
			return nil
		}).
		When(ModeTypeText, func() error {
			NewTextPrinter(printerDat, nil).Flush()
			return nil
		}).
		When(ModeTypeJSON, func() error {
			NewJSONPrinter(printerDat, nil).Flush()
			return nil
		}).Run()
}
