package sdkutil_test

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/sdkutil"
	"github.com/ovrclk/akash/testutil"
)

const akashHexLen = 20
const akashBech32Len = 44

func TestBlockIDStringer(t *testing.T) {
	hexRaw := "3ff68b134903fecb027c97bd05a1e3d54f05fcfc"
	buf, err := hex.DecodeString(hexRaw)
	acc := sdk.AccAddress(buf)
	assert.NoError(t, err)
	d, g, o := uint64(10), uint32(0), uint32(0)
	x := sdkutil.FmtBlockID(&acc, &d, &g, &o, nil)
	assert.Equal(t, x, "akash18lmgky6fq0lvkqnuj77stg0r648stl8ucf0m2z/10/0/0")

	//neh := "211E4C7BB9D12D57E10BE88EB8EE8351031BF14B"
	hexRaw = "a46e1186d64c2e523573bdad70a5832095abdc21"
	buf, err = hex.DecodeString(hexRaw)
	acc = sdk.AccAddress(buf)
	assert.NoError(t, err)
	x = sdkutil.FmtBlockID(&acc, &d, &g, &o, nil)
	assert.Equal(t, x, "akash153hprpkkfsh9ydtnhkkhpfvryz26hhpp67e97k/10/0/0")

	addr := testutil.AccAddress(t)
	dseq := uint64(10)
	gseq := uint32(5)
	oseq := uint32(3)
	provider := testutil.AccAddress(t)
	fmtStr := sdkutil.FmtBlockID(&addr, &dseq, &gseq, &oseq, &provider)

	b, err := sdkutil.ParseBlockID(fmtStr)
	t.Run("parse formatted string", func(t *testing.T) {
		assert.NoError(t, err)
		assert.Equal(t, addr, *b.Owner)
		assert.Equal(t, dseq, *b.DSeq)
		assert.Equal(t, gseq, *b.GSeq)
		assert.Equal(t, oseq, *b.OSeq)
		assert.Equal(t, provider, *b.Provider)
	})

	newBlock := sdkutil.NewBlockID(
		b.Owner,
		b.DSeq,
		b.GSeq,
		b.OSeq,
		b.Provider)
	t.Run("compare block IDs", func(t *testing.T) {
		assert.NoError(t, err)
		assert.Equal(t, *newBlock.Owner, *b.Owner)
		assert.Equal(t, *newBlock.DSeq, *b.DSeq)
		assert.Equal(t, *newBlock.GSeq, *b.GSeq)
		assert.Equal(t, *newBlock.OSeq, *b.OSeq)
		assert.Equal(t, *newBlock.Provider, *b.Provider)

	})
}

func TestErrorCases(t *testing.T) {
	type errCase struct {
		desc        string
		errParseStr string
		expErrIs    error
	}
	//full := "akash19jeunqhqy04wj5ee2t3kq2l6uee53fqkmp878d/1478600512/2170091657/2036973947/akash18u664a4m558u79vecdne93fwd80nhfgd9sv4er"
	tests := []errCase{
		{
			desc:        "error parsing empty values",
			errParseStr: "akash19jeunqhqy04wj5ee2t3kq2l6uee53fqkmp878d/",
		},
		{
			desc:        "error parsing bad owner",
			errParseStr: "hihi",
			expErrIs:    sdkutil.ErrInvalidParseBlockIDInput,
		},
		{
			desc:        "error parsing bad owner field",
			errParseStr: "akash19jeunqhqy04wj5ee2t3kq2l6uee53fqkmp8xxx/1478600512/2170091657/2036973947/akash18u664a4m558u79vecdne93fwd80nhfgd9svxxx",
		},
		{
			desc:        "error parsing owner string size",
			errParseStr: "akash19jeunqhqy04wj5e3/1478600512/2170091657/2036973947/akash18u664a4m558u79vecdne93fwd80nhfgd9sv4er",
		},
		{
			desc:        "error parsing dseq",
			errParseStr: "akash19jeunqhqy04wj5ee2t3kq2l6uee53fqkmp878d/14786005x/2157/20369/akash18u664a4m558u79vecdne93fwd80nhfgd9sv4er",
		},
		{
			desc:        "error parsing empty dseq value",
			errParseStr: "akash19jeunqhqy04wj5ee2t3kq2l6uee53fqkmp878d//2157/20369/akash18u664a4m558u79vecdne93fwd80nhfgd9sv4er",
		},
		{
			desc:        "error parsing gseq",
			errParseStr: "akash19jeunqhqy04wj5ee2t3kq2l6uee53fqkmp878d/147860/2157x/2/akash18u664a4m558u79vecdne93fwd80nhfgd9sv4er",
		},
		{
			desc:        "error parsing oseq",
			errParseStr: "akash19jeunqhqy04wj5ee2t3kq2l6uee53fqkmp878d/1478600512/2170091657/xxx/akash18u664a4m558u79vecdne93fwd80nhfgd9sv4er",
		},
		{
			desc:        "error parsing provider",
			errParseStr: "akash19jeunqhqy04wj5ee2t3kq2l6uee53fqkmp878d/1478600512/2170091657/2036973947/akash18u664a4m558u79vecdne93fwd80nhfgd9svxxx",
		},
	}

	for _, test := range tests {
		t.Log(test.desc)
		b, err := sdkutil.ParseBlockID(test.errParseStr)
		assert.Nil(t, b)
		assert.Error(t, err)
		if test.expErrIs != nil {
			if !errors.Is(err, test.expErrIs) {
				t.Errorf("unexpected error: %v", err)
			}
		}
	}
}

func TestBidID(t *testing.T) {
	bid := testutil.BidID(t)
	block := sdkutil.ReflectBlockID(bid)

	str := block.String()
	block, err := sdkutil.ParseBlockID(str)
	assert.NoError(t, err)

	assert.Equal(t, bid.Owner, *block.Owner)
	assert.Equal(t, bid.DSeq, *block.DSeq)
	assert.Equal(t, bid.GSeq, *block.GSeq)
	assert.Equal(t, bid.OSeq, *block.OSeq)
	assert.Equal(t, bid.Provider, *block.Provider)
}

func TestGroupIDString(t *testing.T) {
	gid := testutil.GroupID(t)
	str := gid.String()

	split := strings.Split(str, "/")
	if len(split) != 3 {
		t.Error("expected returned path have three elements")
	}
}

func TestBlockReflectionID(t *testing.T) {
	t.Run("deployment-id", func(t *testing.T) {
		id := testutil.DeploymentID(t)
		block := sdkutil.ReflectBlockID(id)

		assert.Equal(t, id.Owner, *block.Owner)
		assert.Equal(t, id.DSeq, *block.DSeq)

		pb, err := sdkutil.ParseBlockID(block.String())
		assert.NoError(t, err)
		assert.Equal(t, id.Owner, *pb.Owner)
		assert.Equal(t, id.DSeq, *pb.DSeq)
	})

	t.Run("group-id", func(t *testing.T) {
		id := testutil.GroupID(t)
		block := sdkutil.ReflectBlockID(id)

		assert.Equal(t, id.Owner, *block.Owner)
		assert.Equal(t, id.DSeq, *block.DSeq)
		assert.Equal(t, id.GSeq, *block.GSeq)

		pb, err := sdkutil.ParseBlockID(block.String())
		assert.NoError(t, err)
		assert.Equal(t, id.Owner, *pb.Owner)
		assert.Equal(t, id.DSeq, *pb.DSeq)
		assert.Equal(t, id.GSeq, *pb.GSeq)
	})

	t.Run("order-id", func(t *testing.T) {
		id := testutil.OrderID(t)
		block := sdkutil.ReflectBlockID(id)

		assert.Equal(t, id.Owner, *block.Owner)
		assert.Equal(t, id.DSeq, *block.DSeq)
		assert.Equal(t, id.GSeq, *block.GSeq)
		assert.Equal(t, id.OSeq, *block.OSeq)

		pb, err := sdkutil.ParseBlockID(block.String())
		assert.NoError(t, err)
		assert.Equal(t, id.Owner, *pb.Owner)
		assert.Equal(t, id.DSeq, *pb.DSeq)
		assert.Equal(t, id.GSeq, *pb.GSeq)
		assert.Equal(t, id.OSeq, *pb.OSeq)
	})

	t.Run("lease-id", func(t *testing.T) {
		id := testutil.LeaseID(t)
		block := sdkutil.ReflectBlockID(id)

		assert.Equal(t, id.Owner, *block.Owner)
		assert.Equal(t, id.DSeq, *block.DSeq)
		assert.Equal(t, id.GSeq, *block.GSeq)
		assert.Equal(t, id.OSeq, *block.OSeq)
		assert.Equal(t, id.Provider, *block.Provider)

		pb, err := sdkutil.ParseBlockID(block.String())
		assert.NoError(t, err)
		assert.Equal(t, id.Owner, *pb.Owner)
		assert.Equal(t, id.DSeq, *pb.DSeq)
		assert.Equal(t, id.GSeq, *pb.GSeq)
		assert.Equal(t, id.OSeq, *pb.OSeq)
		assert.Equal(t, id.Provider, *pb.Provider)
	})

	t.Run("bid-id", func(t *testing.T) {
		id := testutil.BidID(t)

		pb, err := sdkutil.ParseBlockID(id.String())
		assert.NoError(t, err)
		assert.Equal(t, id.Owner, *pb.Owner)
		assert.Equal(t, id.DSeq, *pb.DSeq)
		assert.Equal(t, id.GSeq, *pb.GSeq)
		assert.Equal(t, id.OSeq, *pb.OSeq)
		assert.Equal(t, id.Provider, *pb.Provider)

		t.Run("reflect", func(t *testing.T) {
			block := sdkutil.ReflectBlockID(id)
			assert.Equal(t, id.Owner, *block.Owner)
			assert.Equal(t, id.DSeq, *block.DSeq)
			assert.Equal(t, id.GSeq, *block.GSeq)
			assert.Equal(t, id.OSeq, *block.OSeq)
			assert.Equal(t, id.Provider, *block.Provider)
		})

	})
}

func TestAddressLengths(t *testing.T) {
	x := "akash1ncagkrcvqw0nn4jee5j5efl0ylguka2v36t7t7"
	assert.Len(t, x, akashBech32Len)

	y := testutil.AccAddress(t)
	assert.Len(t, y, akashHexLen)
}
