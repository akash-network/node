package sdkutil

import (
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
)

const (
	blockIDOwnerPosition   = 0
	blockIDDSeqPosition    = 1
	blockIDGSeqPosition    = 2
	blockIDOSeqPosition    = 3
	blockIDProviderPostion = 4
)

// BlockID provides type to properly format print blockchain ID types.
type BlockID struct {
	Owner    *sdk.AccAddress
	DSeq     *uint64
	GSeq     *uint32
	OSeq     *uint32
	Provider *sdk.AccAddress
}

// NewBlockID initializes the BlockID struct, passing nil values is acceptable
// for fields which are not used.
func NewBlockID(owner *sdk.AccAddress, dseq *uint64, gseq *uint32, oseq *uint32, provider *sdk.AccAddress) BlockID {
	return BlockID{
		Owner:    owner,
		DSeq:     dseq,
		GSeq:     gseq,
		OSeq:     oseq,
		Provider: provider,
	}
}

// String method provides the full formating of all the BlockID fields which are not nil.
// Format: [Owner AccAddress]/[DSeq]/[GSeq]/[OSeq]/[Provider AccAddress]
// eg: akash1vgv30hr8lel8r7270wywmak5c82es3njws3u4z/795423625/1/1/akash1pxqksfr60zc6uadht6300uavwl3s58ct0apt9w
func (b BlockID) String() string {
	parts := make([]string, 0)
	if b.Owner != nil {
		parts = append(parts, b.Owner.String())
	}
	if b.DSeq != nil {
		parts = append(parts, strconv.FormatUint(*b.DSeq, 10))
	}
	if b.GSeq != nil {
		parts = append(parts, strconv.FormatUint(uint64(*b.GSeq), 10))
	}
	if b.OSeq != nil {
		parts = append(parts, strconv.FormatUint(uint64(*b.OSeq), 10))
	}
	if b.Provider != nil {
		parts = append(parts, b.Provider.String())
	}
	return path.Join(parts...)
}

// ReflectBlockID accepts an ID struct and unpacks the block identifying fields.
// Useful for passing *ID types and getting consistent output based on the consistent
// field names.
func ReflectBlockID(id interface{}) *BlockID {
	b := BlockID{}

	val := reflect.ValueOf(id)
	typeOfID := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		switch typeOfID.Field(i).Name {
		case "Owner":
			if field.Type() == reflect.TypeOf(sdk.AccAddress{}) {
				x, ok := field.Interface().(sdk.AccAddress)
				if ok && len(x) == sdk.AddrLen {
					b.Owner = &x
				}
			}
		case "DSeq":
			if field.Type() == reflect.TypeOf(uint64(0)) {
				x, ok := field.Interface().(uint64)
				if ok {
					b.DSeq = &x
				}
			}
		case "GSeq":
			if field.Type() == reflect.TypeOf(uint32(0)) {
				x, ok := field.Interface().(uint32)
				if ok {
					b.GSeq = &x
				}
			}
		case "OSeq":
			if field.Type() == reflect.TypeOf(uint32(0)) {
				x, ok := field.Interface().(uint32)
				if ok {
					b.OSeq = &x
				}
			}
		case "Provider":
			if field.Type() == reflect.TypeOf(sdk.AccAddress{}) {
				x, ok := field.Interface().(sdk.AccAddress)
				if ok && len(x) == sdk.AddrLen {
					b.Provider = &x
				}
			}
		default:
			// Skip this field
		}
	}
	return &b
}

// FmtBlockID provides a human readable representation of the block chain ID fields.
func FmtBlockID(owner *sdk.AccAddress, dseq *uint64, gseq *uint32, oseq *uint32, provider *sdk.AccAddress) string {
	return BlockID{
		Owner:    owner,
		DSeq:     dseq,
		GSeq:     gseq,
		OSeq:     oseq,
		Provider: provider,
	}.String()
}

// ParseBlockID returns the values from a human readable ID string.
func ParseBlockID(id string) (*BlockID, error) {
	b := &BlockID{}

	parts := strings.Split(id, string(filepath.Separator))
	if len(parts) < 2 {
		return nil, ErrInvalidParseBlockIDInput
	}

	// Owner Address field always expected
	aa, err := sdk.AccAddressFromBech32(parts[blockIDOwnerPosition])
	if err != nil {
		return nil, errors.Wrap(ErrParsingBlockID, err.Error())
	} else if len(aa) > 0 {
		b.Owner = &aa
	}

	if len(parts) > blockIDDSeqPosition { // DSeq
		dseq, err := strconv.ParseUint(parts[blockIDDSeqPosition], 10, 64)
		if err != nil {
			return nil, errors.Wrap(ErrParsingBlockID, err.Error())
		}
		b.DSeq = &dseq
	}

	if len(parts) > blockIDGSeqPosition { // GSeq
		gseq, err := strconv.ParseUint(parts[2], 10, 64)
		if err != nil {
			return nil, errors.Wrap(ErrParsingBlockID, err.Error())
		}
		x := uint32(gseq)
		b.GSeq = &x
	}

	if len(parts) > blockIDOSeqPosition { // OSeq
		oseq, err := strconv.ParseUint(parts[3], 10, 64)
		if err != nil {
			return nil, errors.Wrap(ErrParsingBlockID, err.Error())
		}
		x := uint32(oseq)
		b.OSeq = &x
	}

	if len(parts) > blockIDProviderPostion { // Provider
		pa, err := sdk.AccAddressFromBech32(parts[4])
		if err != nil {
			return nil, errors.Wrap(ErrParsingBlockID, err.Error())
		}
		b.Provider = &pa
	}
	return b, nil
}
