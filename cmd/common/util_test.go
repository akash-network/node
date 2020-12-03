package common

import (
	"github.com/cosmos/cosmos-sdk/client"
	"testing"
)

func TestPrintJSONStdoutStruct(t *testing.T) {
	ctx := client.Context{}
	var x struct{ foo int }
	x.foo = 55
	err := PrintJSON(ctx, x)
	if err != nil {
		t.Errorf("PrintJSON failed:[%T] %v", err, err)
	}
}

func TestPrintJSONStdoutInt(t *testing.T) {
	ctx := client.Context{}
	x := 123
	err := PrintJSON(ctx, x)
	if err != nil {
		t.Errorf("PrintJSON failed:[%T] %v", err, err)
	}
}

func TestPrintJSONStdoutNil(t *testing.T) {
	ctx := client.Context{}
	var x interface{} // implicitly nil
	err := PrintJSON(ctx, x)
	if err != nil {
		t.Errorf("PrintJSON failed:[%T] %v", err, err)
	}
}
