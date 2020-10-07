package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
)

var (
	errOverflow  = errors.Errorf("resource value overflow")
	errCannotSub = errors.Errorf("cannot sub resources when lhs does not have same units as rhs")
)

/*
ResourceValue the big point of this small change is to ensure math operations on resources
not resulting with negative value which panic on unsigned types as well as overflow which leads to panic too
instead reasonable error is returned.
Each resource using this type as value can take extra advantage of it to check upper bounds
For example in SDL v1 CPU units were handled as uint32 and operation like math.MaxUint32 + 2
would cause application to panic. But nowadays
	const CPULimit = math.MaxUint32

	func (c *CPU) add(rhs CPU) error {
		res, err := c.Units.add(rhs.Units)
		if err != nil {
			return err
		}

		if res.Units.Value() > CPULimit {
			return ErrOverflow
		}

		c.Units = res

		return nil
	}
*/

func NewResourceValue(val uint64) ResourceValue {
	res := ResourceValue{
		Val: sdk.NewIntFromUint64(val),
	}

	return res
}

func (m ResourceValue) Value() uint64 {
	return m.Val.Uint64()
}

func (m ResourceValue) equals(rhs ResourceValue) bool {
	return m.Val.Equal(rhs.Val)
}

func (m ResourceValue) le(rhs ResourceValue) bool {
	return m.Val.LTE(rhs.Val)
}

func (m ResourceValue) add(rhs ResourceValue) (ResourceValue, error) {
	res := m.Val
	res = res.Add(rhs.Val)

	if res.Sign() == -1 {
		return ResourceValue{}, errOverflow
	}

	return ResourceValue{res}, nil
}

func (m ResourceValue) sub(rhs ResourceValue) (ResourceValue, error) {
	res := m.Val

	res = res.Sub(rhs.Val)

	if res.Sign() == -1 {
		return ResourceValue{}, errCannotSub
	}

	return ResourceValue{res}, nil
}
