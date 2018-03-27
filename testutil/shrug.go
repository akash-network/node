package testutil

import (
	"fmt"
	"os"
	"testing"
)

const shrug = `¯\_(ツ)_/¯`

func Shrug(t *testing.T, issue int) {
	if os.Getenv("TEST_UNSKIP") == "" {
		t.Skip(fmt.Sprintf("%v - https://github.com/ovrclk/akash/issues/%d", shrug, issue))
	}
}
