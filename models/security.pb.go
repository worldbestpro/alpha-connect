// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: security.proto

package models

import (
	encoding_binary "encoding/binary"
	fmt "fmt"
	proto "github.com/gogo/protobuf/proto"
	types "github.com/gogo/protobuf/types"
	models "gitlab.com/alphaticks/xchanger/models"
	io "io"
	math "math"
	reflect "reflect"
	strings "strings"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion2 // please upgrade the proto package

type Instrument struct {
	SecurityID uint64           `protobuf:"varint,1,opt,name=securityID,proto3" json:"securityID,omitempty"`
	Exchange   *models.Exchange `protobuf:"bytes,2,opt,name=exchange,proto3" json:"exchange,omitempty"`
	Symbol     string           `protobuf:"bytes,3,opt,name=symbol,proto3" json:"symbol,omitempty"`
}

func (m *Instrument) Reset()      { *m = Instrument{} }
func (*Instrument) ProtoMessage() {}
func (*Instrument) Descriptor() ([]byte, []int) {
	return fileDescriptor_55a487c716a8b59c, []int{0}
}
func (m *Instrument) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Instrument) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_Instrument.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalTo(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *Instrument) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Instrument.Merge(m, src)
}
func (m *Instrument) XXX_Size() int {
	return m.Size()
}
func (m *Instrument) XXX_DiscardUnknown() {
	xxx_messageInfo_Instrument.DiscardUnknown(m)
}

var xxx_messageInfo_Instrument proto.InternalMessageInfo

func (m *Instrument) GetSecurityID() uint64 {
	if m != nil {
		return m.SecurityID
	}
	return 0
}

func (m *Instrument) GetExchange() *models.Exchange {
	if m != nil {
		return m.Exchange
	}
	return nil
}

func (m *Instrument) GetSymbol() string {
	if m != nil {
		return m.Symbol
	}
	return ""
}

type Security struct {
	SecurityID        uint64             `protobuf:"varint,1,opt,name=securityID,proto3" json:"securityID,omitempty"`
	SecurityType      string             `protobuf:"bytes,2,opt,name=securityType,proto3" json:"securityType,omitempty"`
	Exchange          *models.Exchange   `protobuf:"bytes,3,opt,name=exchange,proto3" json:"exchange,omitempty"`
	Symbol            string             `protobuf:"bytes,4,opt,name=symbol,proto3" json:"symbol,omitempty"`
	MinPriceIncrement float64            `protobuf:"fixed64,5,opt,name=min_price_increment,json=minPriceIncrement,proto3" json:"min_price_increment,omitempty"`
	RoundLot          float64            `protobuf:"fixed64,6,opt,name=round_lot,json=roundLot,proto3" json:"round_lot,omitempty"`
	Underlying        *models.Asset      `protobuf:"bytes,7,opt,name=underlying,proto3" json:"underlying,omitempty"`
	QuoteCurrency     *models.Asset      `protobuf:"bytes,8,opt,name=quote_currency,json=quoteCurrency,proto3" json:"quote_currency,omitempty"`
	Enabled           bool               `protobuf:"varint,9,opt,name=enabled,proto3" json:"enabled,omitempty"`
	IsInverse         bool               `protobuf:"varint,10,opt,name=is_inverse,json=isInverse,proto3" json:"is_inverse,omitempty"`
	MakerFee          *types.DoubleValue `protobuf:"bytes,11,opt,name=maker_fee,json=makerFee,proto3" json:"maker_fee,omitempty"`
	TakerFee          *types.DoubleValue `protobuf:"bytes,12,opt,name=taker_fee,json=takerFee,proto3" json:"taker_fee,omitempty"`
	Multiplier        *types.DoubleValue `protobuf:"bytes,13,opt,name=multiplier,proto3" json:"multiplier,omitempty"`
	MaturityDate      *types.Timestamp   `protobuf:"bytes,14,opt,name=maturity_date,json=maturityDate,proto3" json:"maturity_date,omitempty"`
}

func (m *Security) Reset()      { *m = Security{} }
func (*Security) ProtoMessage() {}
func (*Security) Descriptor() ([]byte, []int) {
	return fileDescriptor_55a487c716a8b59c, []int{1}
}
func (m *Security) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Security) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_Security.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalTo(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *Security) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Security.Merge(m, src)
}
func (m *Security) XXX_Size() int {
	return m.Size()
}
func (m *Security) XXX_DiscardUnknown() {
	xxx_messageInfo_Security.DiscardUnknown(m)
}

var xxx_messageInfo_Security proto.InternalMessageInfo

func (m *Security) GetSecurityID() uint64 {
	if m != nil {
		return m.SecurityID
	}
	return 0
}

func (m *Security) GetSecurityType() string {
	if m != nil {
		return m.SecurityType
	}
	return ""
}

func (m *Security) GetExchange() *models.Exchange {
	if m != nil {
		return m.Exchange
	}
	return nil
}

func (m *Security) GetSymbol() string {
	if m != nil {
		return m.Symbol
	}
	return ""
}

func (m *Security) GetMinPriceIncrement() float64 {
	if m != nil {
		return m.MinPriceIncrement
	}
	return 0
}

func (m *Security) GetRoundLot() float64 {
	if m != nil {
		return m.RoundLot
	}
	return 0
}

func (m *Security) GetUnderlying() *models.Asset {
	if m != nil {
		return m.Underlying
	}
	return nil
}

func (m *Security) GetQuoteCurrency() *models.Asset {
	if m != nil {
		return m.QuoteCurrency
	}
	return nil
}

func (m *Security) GetEnabled() bool {
	if m != nil {
		return m.Enabled
	}
	return false
}

func (m *Security) GetIsInverse() bool {
	if m != nil {
		return m.IsInverse
	}
	return false
}

func (m *Security) GetMakerFee() *types.DoubleValue {
	if m != nil {
		return m.MakerFee
	}
	return nil
}

func (m *Security) GetTakerFee() *types.DoubleValue {
	if m != nil {
		return m.TakerFee
	}
	return nil
}

func (m *Security) GetMultiplier() *types.DoubleValue {
	if m != nil {
		return m.Multiplier
	}
	return nil
}

func (m *Security) GetMaturityDate() *types.Timestamp {
	if m != nil {
		return m.MaturityDate
	}
	return nil
}

func init() {
	proto.RegisterType((*Instrument)(nil), "models.Instrument")
	proto.RegisterType((*Security)(nil), "models.Security")
}

func init() { proto.RegisterFile("security.proto", fileDescriptor_55a487c716a8b59c) }

var fileDescriptor_55a487c716a8b59c = []byte{
	// 519 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x94, 0x92, 0x41, 0x6f, 0xd3, 0x30,
	0x18, 0x86, 0x63, 0x36, 0xba, 0xc4, 0x5b, 0x27, 0x30, 0x12, 0xb2, 0x06, 0x98, 0xaa, 0xa7, 0x1e,
	0x20, 0x95, 0xc6, 0x2e, 0x48, 0x48, 0x08, 0x28, 0x48, 0x95, 0x38, 0xa0, 0x30, 0x71, 0x8d, 0x9c,
	0xf4, 0x5b, 0x67, 0xcd, 0xb1, 0x83, 0xed, 0x00, 0xbd, 0xf1, 0x13, 0xf8, 0x19, 0xfc, 0x14, 0x8e,
	0x3d, 0xee, 0x48, 0xd3, 0x0b, 0xc7, 0x1d, 0xf8, 0x01, 0x68, 0x4e, 0x33, 0x75, 0x80, 0xc4, 0x38,
	0xfa, 0x7d, 0xdf, 0xe7, 0xfb, 0x5e, 0x25, 0x1f, 0xde, 0xb5, 0x90, 0x57, 0x46, 0xb8, 0x59, 0x5c,
	0x1a, 0xed, 0x34, 0xe9, 0x14, 0x7a, 0x02, 0xd2, 0xee, 0xed, 0x4f, 0x85, 0x93, 0x3c, 0x8b, 0x73,
	0x5d, 0x0c, 0xb9, 0x2c, 0x8f, 0xb9, 0x13, 0xf9, 0x89, 0x1d, 0x7e, 0xca, 0x8f, 0xb9, 0x9a, 0x82,
	0x19, 0x36, 0xb1, 0xa1, 0x87, 0x6c, 0xc3, 0xee, 0xb1, 0xa9, 0xd6, 0x53, 0x09, 0x8d, 0x98, 0x55,
	0x47, 0xc3, 0x8f, 0x86, 0x97, 0x25, 0x98, 0xd6, 0xbf, 0xff, 0xbb, 0xef, 0x44, 0x01, 0xd6, 0xf1,
	0xa2, 0x6c, 0x02, 0x7d, 0x83, 0xf1, 0x58, 0x59, 0x67, 0xaa, 0x02, 0x94, 0x23, 0x0c, 0xe3, 0xb6,
	0xdc, 0x78, 0x44, 0x51, 0x0f, 0x0d, 0x36, 0x93, 0x35, 0x85, 0x3c, 0xc0, 0x21, 0xac, 0xfa, 0xd0,
	0x6b, 0x3d, 0x34, 0xd8, 0xde, 0xbf, 0x11, 0x37, 0xb5, 0xe2, 0x97, 0x2b, 0x3d, 0xb9, 0x48, 0x90,
	0xdb, 0xb8, 0x63, 0x67, 0x45, 0xa6, 0x25, 0xdd, 0xe8, 0xa1, 0x41, 0x94, 0xac, 0x5e, 0xfd, 0x9f,
	0x9b, 0x38, 0x7c, 0xbb, 0x1a, 0xfa, 0xcf, 0x95, 0x7d, 0xbc, 0xd3, 0xbe, 0x0e, 0x67, 0x65, 0xb3,
	0x36, 0x4a, 0x2e, 0x69, 0x97, 0x6a, 0x6d, 0xfc, 0x47, 0xad, 0xcd, 0xf5, 0x5a, 0x24, 0xc6, 0xb7,
	0x0a, 0xa1, 0xd2, 0xd2, 0x88, 0x1c, 0x52, 0xa1, 0x72, 0x03, 0xe7, 0xdf, 0x84, 0x5e, 0xef, 0xa1,
	0x01, 0x4a, 0x6e, 0x16, 0x42, 0xbd, 0x39, 0x77, 0xc6, 0xad, 0x41, 0xee, 0xe0, 0xc8, 0xe8, 0x4a,
	0x4d, 0x52, 0xa9, 0x1d, 0xed, 0xf8, 0x54, 0xe8, 0x85, 0xd7, 0xda, 0x91, 0x87, 0x18, 0x57, 0x6a,
	0x02, 0x46, 0xce, 0x84, 0x9a, 0xd2, 0x2d, 0x5f, 0xaa, 0xdb, 0x96, 0x7a, 0x66, 0x2d, 0xb8, 0x64,
	0x2d, 0x40, 0x0e, 0xf0, 0xee, 0xfb, 0x4a, 0x3b, 0x48, 0xf3, 0xca, 0x18, 0x50, 0xf9, 0x8c, 0x86,
	0x7f, 0x43, 0xba, 0x3e, 0xf4, 0x62, 0x95, 0x21, 0x14, 0x6f, 0x81, 0xe2, 0x99, 0x84, 0x09, 0x8d,
	0x7a, 0x68, 0x10, 0x26, 0xed, 0x93, 0xdc, 0xc3, 0x58, 0xd8, 0x54, 0xa8, 0x0f, 0x60, 0x2c, 0x50,
	0xec, 0xcd, 0x48, 0xd8, 0x71, 0x23, 0x90, 0xc7, 0x38, 0x2a, 0xf8, 0x09, 0x98, 0xf4, 0x08, 0x80,
	0x6e, 0xfb, 0x4d, 0x77, 0xe3, 0xe6, 0x54, 0xe2, 0xf6, 0x54, 0xe2, 0x91, 0xae, 0x32, 0x09, 0xef,
	0xb8, 0xac, 0x20, 0x09, 0x7d, 0xfc, 0x15, 0x78, 0xd4, 0x5d, 0xa0, 0x3b, 0x57, 0x41, 0x5d, 0x8b,
	0x3e, 0xc1, 0xb8, 0xa8, 0xa4, 0x13, 0xa5, 0x14, 0x60, 0x68, 0xf7, 0x0a, 0xec, 0x5a, 0x9e, 0x3c,
	0xc5, 0xdd, 0x82, 0x3b, 0xff, 0xd3, 0xd3, 0x09, 0x77, 0x40, 0x77, 0xfd, 0x80, 0xbd, 0x3f, 0x06,
	0x1c, 0xb6, 0x27, 0x9e, 0xec, 0xb4, 0xc0, 0x88, 0x3b, 0x78, 0x7e, 0x30, 0x5f, 0xb0, 0xe0, 0x74,
	0xc1, 0x82, 0xb3, 0x05, 0x43, 0x9f, 0x6b, 0x86, 0xbe, 0xd6, 0x0c, 0x7d, 0xab, 0x19, 0x9a, 0xd7,
	0x0c, 0x7d, 0xaf, 0x19, 0xfa, 0x51, 0xb3, 0xe0, 0xac, 0x66, 0xe8, 0xcb, 0x92, 0x05, 0xf3, 0x25,
	0x0b, 0x4e, 0x97, 0x2c, 0xc8, 0x3a, 0x7e, 0xee, 0xa3, 0x5f, 0x01, 0x00, 0x00, 0xff, 0xff, 0xc3,
	0x2d, 0x4e, 0x24, 0xb6, 0x03, 0x00, 0x00,
}

func (this *Instrument) Equal(that interface{}) bool {
	if that == nil {
		return this == nil
	}

	that1, ok := that.(*Instrument)
	if !ok {
		that2, ok := that.(Instrument)
		if ok {
			that1 = &that2
		} else {
			return false
		}
	}
	if that1 == nil {
		return this == nil
	} else if this == nil {
		return false
	}
	if this.SecurityID != that1.SecurityID {
		return false
	}
	if !this.Exchange.Equal(that1.Exchange) {
		return false
	}
	if this.Symbol != that1.Symbol {
		return false
	}
	return true
}
func (this *Security) Equal(that interface{}) bool {
	if that == nil {
		return this == nil
	}

	that1, ok := that.(*Security)
	if !ok {
		that2, ok := that.(Security)
		if ok {
			that1 = &that2
		} else {
			return false
		}
	}
	if that1 == nil {
		return this == nil
	} else if this == nil {
		return false
	}
	if this.SecurityID != that1.SecurityID {
		return false
	}
	if this.SecurityType != that1.SecurityType {
		return false
	}
	if !this.Exchange.Equal(that1.Exchange) {
		return false
	}
	if this.Symbol != that1.Symbol {
		return false
	}
	if this.MinPriceIncrement != that1.MinPriceIncrement {
		return false
	}
	if this.RoundLot != that1.RoundLot {
		return false
	}
	if !this.Underlying.Equal(that1.Underlying) {
		return false
	}
	if !this.QuoteCurrency.Equal(that1.QuoteCurrency) {
		return false
	}
	if this.Enabled != that1.Enabled {
		return false
	}
	if this.IsInverse != that1.IsInverse {
		return false
	}
	if !this.MakerFee.Equal(that1.MakerFee) {
		return false
	}
	if !this.TakerFee.Equal(that1.TakerFee) {
		return false
	}
	if !this.Multiplier.Equal(that1.Multiplier) {
		return false
	}
	if !this.MaturityDate.Equal(that1.MaturityDate) {
		return false
	}
	return true
}
func (this *Instrument) GoString() string {
	if this == nil {
		return "nil"
	}
	s := make([]string, 0, 7)
	s = append(s, "&models.Instrument{")
	s = append(s, "SecurityID: "+fmt.Sprintf("%#v", this.SecurityID)+",\n")
	if this.Exchange != nil {
		s = append(s, "Exchange: "+fmt.Sprintf("%#v", this.Exchange)+",\n")
	}
	s = append(s, "Symbol: "+fmt.Sprintf("%#v", this.Symbol)+",\n")
	s = append(s, "}")
	return strings.Join(s, "")
}
func (this *Security) GoString() string {
	if this == nil {
		return "nil"
	}
	s := make([]string, 0, 18)
	s = append(s, "&models.Security{")
	s = append(s, "SecurityID: "+fmt.Sprintf("%#v", this.SecurityID)+",\n")
	s = append(s, "SecurityType: "+fmt.Sprintf("%#v", this.SecurityType)+",\n")
	if this.Exchange != nil {
		s = append(s, "Exchange: "+fmt.Sprintf("%#v", this.Exchange)+",\n")
	}
	s = append(s, "Symbol: "+fmt.Sprintf("%#v", this.Symbol)+",\n")
	s = append(s, "MinPriceIncrement: "+fmt.Sprintf("%#v", this.MinPriceIncrement)+",\n")
	s = append(s, "RoundLot: "+fmt.Sprintf("%#v", this.RoundLot)+",\n")
	if this.Underlying != nil {
		s = append(s, "Underlying: "+fmt.Sprintf("%#v", this.Underlying)+",\n")
	}
	if this.QuoteCurrency != nil {
		s = append(s, "QuoteCurrency: "+fmt.Sprintf("%#v", this.QuoteCurrency)+",\n")
	}
	s = append(s, "Enabled: "+fmt.Sprintf("%#v", this.Enabled)+",\n")
	s = append(s, "IsInverse: "+fmt.Sprintf("%#v", this.IsInverse)+",\n")
	if this.MakerFee != nil {
		s = append(s, "MakerFee: "+fmt.Sprintf("%#v", this.MakerFee)+",\n")
	}
	if this.TakerFee != nil {
		s = append(s, "TakerFee: "+fmt.Sprintf("%#v", this.TakerFee)+",\n")
	}
	if this.Multiplier != nil {
		s = append(s, "Multiplier: "+fmt.Sprintf("%#v", this.Multiplier)+",\n")
	}
	if this.MaturityDate != nil {
		s = append(s, "MaturityDate: "+fmt.Sprintf("%#v", this.MaturityDate)+",\n")
	}
	s = append(s, "}")
	return strings.Join(s, "")
}
func valueToGoStringSecurity(v interface{}, typ string) string {
	rv := reflect.ValueOf(v)
	if rv.IsNil() {
		return "nil"
	}
	pv := reflect.Indirect(rv).Interface()
	return fmt.Sprintf("func(v %v) *%v { return &v } ( %#v )", typ, typ, pv)
}
func (m *Instrument) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Instrument) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if m.SecurityID != 0 {
		dAtA[i] = 0x8
		i++
		i = encodeVarintSecurity(dAtA, i, uint64(m.SecurityID))
	}
	if m.Exchange != nil {
		dAtA[i] = 0x12
		i++
		i = encodeVarintSecurity(dAtA, i, uint64(m.Exchange.Size()))
		n1, err := m.Exchange.MarshalTo(dAtA[i:])
		if err != nil {
			return 0, err
		}
		i += n1
	}
	if len(m.Symbol) > 0 {
		dAtA[i] = 0x1a
		i++
		i = encodeVarintSecurity(dAtA, i, uint64(len(m.Symbol)))
		i += copy(dAtA[i:], m.Symbol)
	}
	return i, nil
}

func (m *Security) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Security) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if m.SecurityID != 0 {
		dAtA[i] = 0x8
		i++
		i = encodeVarintSecurity(dAtA, i, uint64(m.SecurityID))
	}
	if len(m.SecurityType) > 0 {
		dAtA[i] = 0x12
		i++
		i = encodeVarintSecurity(dAtA, i, uint64(len(m.SecurityType)))
		i += copy(dAtA[i:], m.SecurityType)
	}
	if m.Exchange != nil {
		dAtA[i] = 0x1a
		i++
		i = encodeVarintSecurity(dAtA, i, uint64(m.Exchange.Size()))
		n2, err := m.Exchange.MarshalTo(dAtA[i:])
		if err != nil {
			return 0, err
		}
		i += n2
	}
	if len(m.Symbol) > 0 {
		dAtA[i] = 0x22
		i++
		i = encodeVarintSecurity(dAtA, i, uint64(len(m.Symbol)))
		i += copy(dAtA[i:], m.Symbol)
	}
	if m.MinPriceIncrement != 0 {
		dAtA[i] = 0x29
		i++
		encoding_binary.LittleEndian.PutUint64(dAtA[i:], uint64(math.Float64bits(float64(m.MinPriceIncrement))))
		i += 8
	}
	if m.RoundLot != 0 {
		dAtA[i] = 0x31
		i++
		encoding_binary.LittleEndian.PutUint64(dAtA[i:], uint64(math.Float64bits(float64(m.RoundLot))))
		i += 8
	}
	if m.Underlying != nil {
		dAtA[i] = 0x3a
		i++
		i = encodeVarintSecurity(dAtA, i, uint64(m.Underlying.Size()))
		n3, err := m.Underlying.MarshalTo(dAtA[i:])
		if err != nil {
			return 0, err
		}
		i += n3
	}
	if m.QuoteCurrency != nil {
		dAtA[i] = 0x42
		i++
		i = encodeVarintSecurity(dAtA, i, uint64(m.QuoteCurrency.Size()))
		n4, err := m.QuoteCurrency.MarshalTo(dAtA[i:])
		if err != nil {
			return 0, err
		}
		i += n4
	}
	if m.Enabled {
		dAtA[i] = 0x48
		i++
		if m.Enabled {
			dAtA[i] = 1
		} else {
			dAtA[i] = 0
		}
		i++
	}
	if m.IsInverse {
		dAtA[i] = 0x50
		i++
		if m.IsInverse {
			dAtA[i] = 1
		} else {
			dAtA[i] = 0
		}
		i++
	}
	if m.MakerFee != nil {
		dAtA[i] = 0x5a
		i++
		i = encodeVarintSecurity(dAtA, i, uint64(m.MakerFee.Size()))
		n5, err := m.MakerFee.MarshalTo(dAtA[i:])
		if err != nil {
			return 0, err
		}
		i += n5
	}
	if m.TakerFee != nil {
		dAtA[i] = 0x62
		i++
		i = encodeVarintSecurity(dAtA, i, uint64(m.TakerFee.Size()))
		n6, err := m.TakerFee.MarshalTo(dAtA[i:])
		if err != nil {
			return 0, err
		}
		i += n6
	}
	if m.Multiplier != nil {
		dAtA[i] = 0x6a
		i++
		i = encodeVarintSecurity(dAtA, i, uint64(m.Multiplier.Size()))
		n7, err := m.Multiplier.MarshalTo(dAtA[i:])
		if err != nil {
			return 0, err
		}
		i += n7
	}
	if m.MaturityDate != nil {
		dAtA[i] = 0x72
		i++
		i = encodeVarintSecurity(dAtA, i, uint64(m.MaturityDate.Size()))
		n8, err := m.MaturityDate.MarshalTo(dAtA[i:])
		if err != nil {
			return 0, err
		}
		i += n8
	}
	return i, nil
}

func encodeVarintSecurity(dAtA []byte, offset int, v uint64) int {
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return offset + 1
}
func (m *Instrument) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.SecurityID != 0 {
		n += 1 + sovSecurity(uint64(m.SecurityID))
	}
	if m.Exchange != nil {
		l = m.Exchange.Size()
		n += 1 + l + sovSecurity(uint64(l))
	}
	l = len(m.Symbol)
	if l > 0 {
		n += 1 + l + sovSecurity(uint64(l))
	}
	return n
}

func (m *Security) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.SecurityID != 0 {
		n += 1 + sovSecurity(uint64(m.SecurityID))
	}
	l = len(m.SecurityType)
	if l > 0 {
		n += 1 + l + sovSecurity(uint64(l))
	}
	if m.Exchange != nil {
		l = m.Exchange.Size()
		n += 1 + l + sovSecurity(uint64(l))
	}
	l = len(m.Symbol)
	if l > 0 {
		n += 1 + l + sovSecurity(uint64(l))
	}
	if m.MinPriceIncrement != 0 {
		n += 9
	}
	if m.RoundLot != 0 {
		n += 9
	}
	if m.Underlying != nil {
		l = m.Underlying.Size()
		n += 1 + l + sovSecurity(uint64(l))
	}
	if m.QuoteCurrency != nil {
		l = m.QuoteCurrency.Size()
		n += 1 + l + sovSecurity(uint64(l))
	}
	if m.Enabled {
		n += 2
	}
	if m.IsInverse {
		n += 2
	}
	if m.MakerFee != nil {
		l = m.MakerFee.Size()
		n += 1 + l + sovSecurity(uint64(l))
	}
	if m.TakerFee != nil {
		l = m.TakerFee.Size()
		n += 1 + l + sovSecurity(uint64(l))
	}
	if m.Multiplier != nil {
		l = m.Multiplier.Size()
		n += 1 + l + sovSecurity(uint64(l))
	}
	if m.MaturityDate != nil {
		l = m.MaturityDate.Size()
		n += 1 + l + sovSecurity(uint64(l))
	}
	return n
}

func sovSecurity(x uint64) (n int) {
	for {
		n++
		x >>= 7
		if x == 0 {
			break
		}
	}
	return n
}
func sozSecurity(x uint64) (n int) {
	return sovSecurity(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (this *Instrument) String() string {
	if this == nil {
		return "nil"
	}
	s := strings.Join([]string{`&Instrument{`,
		`SecurityID:` + fmt.Sprintf("%v", this.SecurityID) + `,`,
		`Exchange:` + strings.Replace(fmt.Sprintf("%v", this.Exchange), "Exchange", "models.Exchange", 1) + `,`,
		`Symbol:` + fmt.Sprintf("%v", this.Symbol) + `,`,
		`}`,
	}, "")
	return s
}
func (this *Security) String() string {
	if this == nil {
		return "nil"
	}
	s := strings.Join([]string{`&Security{`,
		`SecurityID:` + fmt.Sprintf("%v", this.SecurityID) + `,`,
		`SecurityType:` + fmt.Sprintf("%v", this.SecurityType) + `,`,
		`Exchange:` + strings.Replace(fmt.Sprintf("%v", this.Exchange), "Exchange", "models.Exchange", 1) + `,`,
		`Symbol:` + fmt.Sprintf("%v", this.Symbol) + `,`,
		`MinPriceIncrement:` + fmt.Sprintf("%v", this.MinPriceIncrement) + `,`,
		`RoundLot:` + fmt.Sprintf("%v", this.RoundLot) + `,`,
		`Underlying:` + strings.Replace(fmt.Sprintf("%v", this.Underlying), "Asset", "models.Asset", 1) + `,`,
		`QuoteCurrency:` + strings.Replace(fmt.Sprintf("%v", this.QuoteCurrency), "Asset", "models.Asset", 1) + `,`,
		`Enabled:` + fmt.Sprintf("%v", this.Enabled) + `,`,
		`IsInverse:` + fmt.Sprintf("%v", this.IsInverse) + `,`,
		`MakerFee:` + strings.Replace(fmt.Sprintf("%v", this.MakerFee), "DoubleValue", "types.DoubleValue", 1) + `,`,
		`TakerFee:` + strings.Replace(fmt.Sprintf("%v", this.TakerFee), "DoubleValue", "types.DoubleValue", 1) + `,`,
		`Multiplier:` + strings.Replace(fmt.Sprintf("%v", this.Multiplier), "DoubleValue", "types.DoubleValue", 1) + `,`,
		`MaturityDate:` + strings.Replace(fmt.Sprintf("%v", this.MaturityDate), "Timestamp", "types.Timestamp", 1) + `,`,
		`}`,
	}, "")
	return s
}
func valueToStringSecurity(v interface{}) string {
	rv := reflect.ValueOf(v)
	if rv.IsNil() {
		return "nil"
	}
	pv := reflect.Indirect(rv).Interface()
	return fmt.Sprintf("*%v", pv)
}
func (m *Instrument) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowSecurity
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
			return fmt.Errorf("proto: Instrument: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Instrument: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field SecurityID", wireType)
			}
			m.SecurityID = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSecurity
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.SecurityID |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Exchange", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSecurity
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
				return ErrInvalidLengthSecurity
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthSecurity
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Exchange == nil {
				m.Exchange = &models.Exchange{}
			}
			if err := m.Exchange.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 3:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Symbol", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSecurity
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthSecurity
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthSecurity
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Symbol = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipSecurity(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthSecurity
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthSecurity
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
func (m *Security) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowSecurity
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
			return fmt.Errorf("proto: Security: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Security: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field SecurityID", wireType)
			}
			m.SecurityID = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSecurity
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.SecurityID |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field SecurityType", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSecurity
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthSecurity
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthSecurity
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.SecurityType = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 3:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Exchange", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSecurity
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
				return ErrInvalidLengthSecurity
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthSecurity
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Exchange == nil {
				m.Exchange = &models.Exchange{}
			}
			if err := m.Exchange.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 4:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Symbol", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSecurity
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthSecurity
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthSecurity
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Symbol = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 5:
			if wireType != 1 {
				return fmt.Errorf("proto: wrong wireType = %d for field MinPriceIncrement", wireType)
			}
			var v uint64
			if (iNdEx + 8) > l {
				return io.ErrUnexpectedEOF
			}
			v = uint64(encoding_binary.LittleEndian.Uint64(dAtA[iNdEx:]))
			iNdEx += 8
			m.MinPriceIncrement = float64(math.Float64frombits(v))
		case 6:
			if wireType != 1 {
				return fmt.Errorf("proto: wrong wireType = %d for field RoundLot", wireType)
			}
			var v uint64
			if (iNdEx + 8) > l {
				return io.ErrUnexpectedEOF
			}
			v = uint64(encoding_binary.LittleEndian.Uint64(dAtA[iNdEx:]))
			iNdEx += 8
			m.RoundLot = float64(math.Float64frombits(v))
		case 7:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Underlying", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSecurity
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
				return ErrInvalidLengthSecurity
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthSecurity
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Underlying == nil {
				m.Underlying = &models.Asset{}
			}
			if err := m.Underlying.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 8:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field QuoteCurrency", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSecurity
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
				return ErrInvalidLengthSecurity
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthSecurity
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.QuoteCurrency == nil {
				m.QuoteCurrency = &models.Asset{}
			}
			if err := m.QuoteCurrency.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 9:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Enabled", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSecurity
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.Enabled = bool(v != 0)
		case 10:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field IsInverse", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSecurity
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.IsInverse = bool(v != 0)
		case 11:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field MakerFee", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSecurity
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
				return ErrInvalidLengthSecurity
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthSecurity
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.MakerFee == nil {
				m.MakerFee = &types.DoubleValue{}
			}
			if err := m.MakerFee.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 12:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field TakerFee", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSecurity
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
				return ErrInvalidLengthSecurity
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthSecurity
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.TakerFee == nil {
				m.TakerFee = &types.DoubleValue{}
			}
			if err := m.TakerFee.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 13:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Multiplier", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSecurity
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
				return ErrInvalidLengthSecurity
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthSecurity
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Multiplier == nil {
				m.Multiplier = &types.DoubleValue{}
			}
			if err := m.Multiplier.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 14:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field MaturityDate", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSecurity
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
				return ErrInvalidLengthSecurity
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthSecurity
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.MaturityDate == nil {
				m.MaturityDate = &types.Timestamp{}
			}
			if err := m.MaturityDate.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipSecurity(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthSecurity
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthSecurity
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
func skipSecurity(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowSecurity
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
					return 0, ErrIntOverflowSecurity
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
			return iNdEx, nil
		case 1:
			iNdEx += 8
			return iNdEx, nil
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowSecurity
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
				return 0, ErrInvalidLengthSecurity
			}
			iNdEx += length
			if iNdEx < 0 {
				return 0, ErrInvalidLengthSecurity
			}
			return iNdEx, nil
		case 3:
			for {
				var innerWire uint64
				var start int = iNdEx
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, ErrIntOverflowSecurity
					}
					if iNdEx >= l {
						return 0, io.ErrUnexpectedEOF
					}
					b := dAtA[iNdEx]
					iNdEx++
					innerWire |= (uint64(b) & 0x7F) << shift
					if b < 0x80 {
						break
					}
				}
				innerWireType := int(innerWire & 0x7)
				if innerWireType == 4 {
					break
				}
				next, err := skipSecurity(dAtA[start:])
				if err != nil {
					return 0, err
				}
				iNdEx = start + next
				if iNdEx < 0 {
					return 0, ErrInvalidLengthSecurity
				}
			}
			return iNdEx, nil
		case 4:
			return iNdEx, nil
		case 5:
			iNdEx += 4
			return iNdEx, nil
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
	}
	panic("unreachable")
}

var (
	ErrInvalidLengthSecurity = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowSecurity   = fmt.Errorf("proto: integer overflow")
)
