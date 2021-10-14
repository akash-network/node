// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: akash/market/v1beta2/params.proto

package v1beta2

import (
	fmt "fmt"
	types "github.com/cosmos/cosmos-sdk/types"
	_ "github.com/gogo/protobuf/gogoproto"
	proto "github.com/gogo/protobuf/proto"
	io "io"
	math "math"
	math_bits "math/bits"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion3 // please upgrade the proto package

// Params is the params for the x/market module
type Params struct {
	BidMinDeposit types.Coin `protobuf:"bytes,1,opt,name=bid_min_deposit,json=bidMinDeposit,proto3" json:"bid_min_deposit" yaml:"bid_min_deposit"`
	OrderMaxBids  uint32     `protobuf:"varint,2,opt,name=order_max_bids,json=orderMaxBids,proto3" json:"order_max_bids" yaml:"order_max_bids"`
}

func (m *Params) Reset()         { *m = Params{} }
func (m *Params) String() string { return proto.CompactTextString(m) }
func (*Params) ProtoMessage()    {}
func (*Params) Descriptor() ([]byte, []int) {
	return fileDescriptor_ea1237af8227f99c, []int{0}
}
func (m *Params) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Params) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_Params.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *Params) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Params.Merge(m, src)
}
func (m *Params) XXX_Size() int {
	return m.Size()
}
func (m *Params) XXX_DiscardUnknown() {
	xxx_messageInfo_Params.DiscardUnknown(m)
}

var xxx_messageInfo_Params proto.InternalMessageInfo

func (m *Params) GetBidMinDeposit() types.Coin {
	if m != nil {
		return m.BidMinDeposit
	}
	return types.Coin{}
}

func (m *Params) GetOrderMaxBids() uint32 {
	if m != nil {
		return m.OrderMaxBids
	}
	return 0
}

func init() {
	proto.RegisterType((*Params)(nil), "akash.market.v1beta2.Params")
}

func init() { proto.RegisterFile("akash/market/v1beta2/params.proto", fileDescriptor_ea1237af8227f99c) }

var fileDescriptor_ea1237af8227f99c = []byte{
	// 316 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x5c, 0x91, 0xb1, 0x6e, 0xea, 0x30,
	0x18, 0x85, 0xe3, 0x3b, 0x30, 0xe4, 0x42, 0x2b, 0x21, 0x5a, 0x51, 0x06, 0x9b, 0x66, 0x62, 0xb2,
	0x05, 0xdd, 0xda, 0x2d, 0xed, 0xd0, 0x05, 0xb5, 0x62, 0xec, 0x12, 0xd9, 0x49, 0x04, 0x16, 0x38,
	0x7f, 0x14, 0xbb, 0x08, 0x1e, 0xa0, 0x7b, 0x1f, 0x8b, 0x91, 0xb1, 0x93, 0x55, 0x85, 0x8d, 0x91,
	0xbe, 0x40, 0x45, 0x0c, 0x12, 0x65, 0x8b, 0x72, 0xbe, 0xf3, 0xfd, 0x92, 0x8f, 0x7f, 0xcb, 0xa7,
	0x5c, 0x4f, 0x98, 0xe2, 0xc5, 0x34, 0x35, 0x6c, 0xde, 0x17, 0xa9, 0xe1, 0x03, 0x96, 0xf3, 0x82,
	0x2b, 0x4d, 0xf3, 0x02, 0x0c, 0x34, 0x5b, 0x15, 0x42, 0x1d, 0x42, 0x0f, 0x48, 0xa7, 0x35, 0x86,
	0x31, 0x54, 0x00, 0xdb, 0x7f, 0x39, 0xb6, 0x83, 0x63, 0xd0, 0x0a, 0x34, 0x13, 0x5c, 0xa7, 0x07,
	0x5b, 0x9f, 0xc5, 0x20, 0x33, 0x97, 0x07, 0x3f, 0xc8, 0xaf, 0xbd, 0x56, 0xf2, 0xe6, 0x07, 0xf2,
	0x2f, 0x85, 0x4c, 0x22, 0x25, 0xb3, 0x28, 0x49, 0x73, 0xd0, 0xd2, 0xb4, 0x51, 0x17, 0xf5, 0xfe,
	0x0f, 0x6e, 0xa8, 0xb3, 0xd0, 0xbd, 0xe5, 0x70, 0xb0, 0x4f, 0x1f, 0x41, 0x66, 0x61, 0xb8, 0xb2,
	0xc4, 0x2b, 0x2d, 0x69, 0x84, 0x32, 0x19, 0xca, 0xec, 0xc9, 0xf5, 0xb6, 0x96, 0x9c, 0xab, 0x76,
	0x96, 0x5c, 0x2f, 0xb9, 0x9a, 0xdd, 0x07, 0x67, 0x41, 0x30, 0x6a, 0x88, 0xd3, 0x6e, 0x93, 0xfb,
	0x17, 0x50, 0x24, 0x69, 0x11, 0x29, 0xbe, 0x88, 0x84, 0x4c, 0x74, 0xfb, 0x5f, 0x17, 0xf5, 0x1a,
	0xe1, 0x43, 0x69, 0x49, 0xfd, 0x65, 0x9f, 0x0c, 0xf9, 0x22, 0x94, 0x89, 0xde, 0x5a, 0x72, 0x46,
	0xee, 0x2c, 0xb9, 0x72, 0x47, 0xfe, 0xfe, 0x0f, 0x46, 0x75, 0x38, 0x29, 0x86, 0xcf, 0xab, 0x12,
	0xa3, 0x75, 0x89, 0xd1, 0x77, 0x89, 0xd1, 0xe7, 0x06, 0x7b, 0xeb, 0x0d, 0xf6, 0xbe, 0x36, 0xd8,
	0x7b, 0xa3, 0x63, 0x69, 0x26, 0xef, 0x82, 0xc6, 0xa0, 0x18, 0xcc, 0x8b, 0x78, 0x36, 0x65, 0x6e,
	0x90, 0xc5, 0x71, 0x12, 0xb3, 0xcc, 0x53, 0x7d, 0x1c, 0x46, 0xd4, 0xaa, 0x67, 0xbc, 0xfb, 0x0d,
	0x00, 0x00, 0xff, 0xff, 0x9d, 0x61, 0xbb, 0x00, 0xb7, 0x01, 0x00, 0x00,
}

func (m *Params) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Params) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *Params) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.OrderMaxBids != 0 {
		i = encodeVarintParams(dAtA, i, uint64(m.OrderMaxBids))
		i--
		dAtA[i] = 0x10
	}
	{
		size, err := m.BidMinDeposit.MarshalToSizedBuffer(dAtA[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarintParams(dAtA, i, uint64(size))
	}
	i--
	dAtA[i] = 0xa
	return len(dAtA) - i, nil
}

func encodeVarintParams(dAtA []byte, offset int, v uint64) int {
	offset -= sovParams(v)
	base := offset
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return base
}
func (m *Params) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = m.BidMinDeposit.Size()
	n += 1 + l + sovParams(uint64(l))
	if m.OrderMaxBids != 0 {
		n += 1 + sovParams(uint64(m.OrderMaxBids))
	}
	return n
}

func sovParams(x uint64) (n int) {
	return (math_bits.Len64(x|1) + 6) / 7
}
func sozParams(x uint64) (n int) {
	return sovParams(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *Params) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowParams
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: Params: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Params: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field BidMinDeposit", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowParams
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthParams
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthParams
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if err := m.BidMinDeposit.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 2:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field OrderMaxBids", wireType)
			}
			m.OrderMaxBids = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowParams
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.OrderMaxBids |= uint32(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		default:
			iNdEx = preIndex
			skippy, err := skipParams(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthParams
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func skipParams(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	depth := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowParams
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowParams
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
		case 1:
			iNdEx += 8
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowParams
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if length < 0 {
				return 0, ErrInvalidLengthParams
			}
			iNdEx += length
		case 3:
			depth++
		case 4:
			if depth == 0 {
				return 0, ErrUnexpectedEndOfGroupParams
			}
			depth--
		case 5:
			iNdEx += 4
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
		if iNdEx < 0 {
			return 0, ErrInvalidLengthParams
		}
		if depth == 0 {
			return iNdEx, nil
		}
	}
	return 0, io.ErrUnexpectedEOF
}

var (
	ErrInvalidLengthParams        = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowParams          = fmt.Errorf("proto: integer overflow")
	ErrUnexpectedEndOfGroupParams = fmt.Errorf("proto: unexpected end of group")
)
