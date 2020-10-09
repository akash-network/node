package common

import (
	"testing"
)

func TestPrintJSONStdoutStruct(t *testing.T) {
	var x struct{ foo int }
	x.foo = 55
	err := PrintJSONStdout(x)
	if err != nil {
		t.Errorf("PrintJSONStdout failed:[%T] %v", err, err)
	}
}

func TestPrintJSONStdoutInt(t *testing.T) {
	x := 123
	err := PrintJSONStdout(x)
	if err != nil {
		t.Errorf("PrintJSONStdout failed:[%T] %v", err, err)
	}
}

func TestPrintJSONStdoutNil(t *testing.T) {
	var x interface{} // implicitly nil
	err := PrintJSONStdout(x)
	if err != nil {
		t.Errorf("PrintJSONStdout failed:[%T] %v", err, err)
	}
}
