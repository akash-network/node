package js

import (
	"bufio"
	"bytes"

	"github.com/ovrclk/gestalt"
	"github.com/ovrclk/gestalt/exec"
)

func Do(matches ...Match) exec.CmdFn {
	return func(b *bufio.Reader, e gestalt.Evaluator) error {
		buf := new(bytes.Buffer)
		if _, err := b.WriteTo(buf); err != nil {
			return err
		}
		for _, match := range matches {
			if err := match.Eval(buf.Bytes(), e); err != nil {
				return err
			}
		}
		return nil
	}
}
