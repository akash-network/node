package js

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/buger/jsonparser"
	"github.com/ovrclk/gestalt"
	"github.com/ovrclk/gestalt/vars"
)

func Str(val string, path ...string) ScalarMatch {
	return String(val, path...)
}

func Int(val int64, path ...string) ScalarMatch {
	return Integer(val, path...)
}

type Match interface {
	Eval([]byte, gestalt.Evaluator) error
}

type ScalarMatch interface {
	Match
	Export(string) ScalarMatch
}

type stringMatch struct {
	val    string
	path   []string
	export string
}

func String(val string, path ...string) ScalarMatch {
	return &stringMatch{
		val:  val,
		path: path,
	}
}

func (m *stringMatch) Export(as string) ScalarMatch {
	m.export = as
	return m
}

func (m *stringMatch) Eval(buf []byte, e gestalt.Evaluator) error {

	path := vars.ExpandAll(e.Vars(), m.path)

	val, err := jsonparser.GetString(buf, path...)

	if err != nil {
		return err
	}

	expect := vars.Expand(e.Vars(), m.val)

	if val != expect {
		return fmt.Errorf("%v: received %v expected %v", strings.Join(path, "."), val, expect)
	}

	if m.export != "" {
		e.Emit(m.export, val)
	}

	return nil
}

type integerMatch struct {
	val    int64
	path   []string
	export string
}

func Integer(val int64, path ...string) ScalarMatch {
	return &integerMatch{
		val:  val,
		path: path,
	}
}

func (m *integerMatch) Export(as string) ScalarMatch {
	m.export = as
	return m
}

func (m *integerMatch) Eval(buf []byte, e gestalt.Evaluator) error {

	path := vars.ExpandAll(e.Vars(), m.path)

	val, err := jsonparser.GetInt(buf, path...)
	if err != nil {
		return err
	}

	if val != m.val {
		return fmt.Errorf("%v: received %v expected %v", strings.Join(path, "."), val, m.val)
	}

	if m.export != "" {
		e.Emit(m.export, strconv.FormatInt(val, 10))
	}

	return nil
}

type anyMatch struct {
	path   []string
	export string
}

func Any(path ...string) ScalarMatch {
	return &anyMatch{
		path: path,
	}
}

func (m *anyMatch) Export(as string) ScalarMatch {
	m.export = as
	return m
}

func (m *anyMatch) Eval(buf []byte, e gestalt.Evaluator) error {

	path := vars.ExpandAll(e.Vars(), m.path)

	val, _, _, err := jsonparser.Get(buf, path...)
	if err != nil {
		return err
	}

	if m.export != "" {
		e.Emit(m.export, string(val))
	}

	return nil
}
