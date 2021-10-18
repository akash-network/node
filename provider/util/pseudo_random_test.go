package util_test

import (
	"github.com/ovrclk/akash/provider/util"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestPseudoRandomFromAddress(t *testing.T) {

	result := util.PseudoRandomUintFromAddr(testutil.AccAddress(t).String(), 1333)
	assert.GreaterOrEqual(t, result, uint(0))
	assert.Less(t, result, uint(1333))

	result = util.PseudoRandomUintFromAddr(testutil.AccAddress(t).String(), 100000)
	assert.GreaterOrEqual(t, result, uint(0))
	assert.Less(t, result, uint(100000))

	result = util.PseudoRandomUintFromAddr("akash1reeywehc76ndcd3gkzaz3yasdk7nfxc0vugnaj", 10000)
	assert.Equal(t, uint(0x8ad), result)

	result = util.PseudoRandomUintFromAddr("akash1c3sycef9tly37tdgq55l6u647d3kst794x2r6g", 10000)
	assert.Equal(t, uint(0xb18), result)
}
