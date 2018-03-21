package testutil

import (
	"fmt"
	"testing"
)

const shrug = `¯\_(ツ)_/¯`

func Shrug(t *testing.T, issue int) {
	t.Skip(fmt.Sprintf("%v - https://github.com/ovrclk/akash/issues/%d", shrug, issue))
}
