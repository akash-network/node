package js

import (
	"bufio"
	"bytes"
	"fmt"

	"github.com/buger/jsonparser"
	"github.com/ovrclk/gestalt"
	"github.com/ovrclk/gestalt/exec"
	"github.com/ovrclk/gestalt/vars"
)

func PathEQInt(expect int, path ...string) exec.CmdFn {
	return func(b *bufio.Reader, e gestalt.Evaluator) error {
		buf := new(bytes.Buffer)
		if _, err := b.WriteTo(buf); err != nil {
			return err
		}

		val, err := jsonparser.GetInt(buf.Bytes(), path...)
		if err != nil {
			return err
		}

		if val != int64(expect) {
			return fmt.Errorf("received %v expected %v", val, expect)
		}
		return nil
	}
}

func PathEQStr(expect string, path ...string) exec.CmdFn {
	return func(b *bufio.Reader, e gestalt.Evaluator) error {
		buf := new(bytes.Buffer)
		if _, err := b.WriteTo(buf); err != nil {
			return err
		}

		val, err := jsonparser.GetString(buf.Bytes(), path...)
		if err != nil {
			return err
		}

		expect = vars.Expand(e.Vars(), expect)

		if val != expect {
			return fmt.Errorf("received %v expected %v", val, expect)
		}
		return nil
	}
}
