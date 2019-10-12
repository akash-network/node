package uiutil

import (
	"os"
	"testing"
)

func TestTitle_String(t *testing.T) {
	t.Skip()
	got := string(NewPrinter(os.Stdout).AddTitle("foo").AddTitle("bar").Bytes())
	expect := "foo\n===\n\nbar\n===\n\n"
	if got != expect {
		t.Fatal("== expected\n", expect, "== got\n", got)
	}
}
