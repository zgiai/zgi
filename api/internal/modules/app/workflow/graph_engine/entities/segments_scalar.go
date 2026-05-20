package entities

import (
	"fmt"
	"unsafe"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

type StringSegment struct {
	Value string `json:"value"`
}

func (s *StringSegment) ToObject() any {
	return s.Value
}

func (s *StringSegment) GetValue() any {
	return s.Value
}

func (s *StringSegment) GetType() shared.SegmentType {
	return shared.SegmentTypeString
}

func (s *StringSegment) Text() string {
	return s.Value
}

func (s *StringSegment) Log() string {
	return s.Value
}

func (s *StringSegment) Markdown() string {
	return s.Value
}

func (s *StringSegment) Size() int {
	return len(s.Value)
}

// SecretSegment secret segment implementation
type SecretSegment struct {
	Value string `json:"value"`
}

func (s *SecretSegment) ToObject() any {
	return s.Value
}

func (s *SecretSegment) GetValue() any {
	return s.Value
}

func (s *SecretSegment) GetType() shared.SegmentType {
	return shared.SegmentTypeSecret
}

func (s *SecretSegment) Text() string {
	return s.Value
}

func (s *SecretSegment) Log() string {
	// Hide sensitive information
	return "****"
}

func (s *SecretSegment) Markdown() string {
	// Hide sensitive information
	return "****"
}

func (s *SecretSegment) Size() int {
	return len(s.Value)
}

type NumberSegment struct {
	Value float64 `json:"value"`
}

func (n *NumberSegment) ToObject() any {
	return n.Value
}

func (n *NumberSegment) GetValue() any {
	return n.Value
}

func (n *NumberSegment) GetType() shared.SegmentType {
	return shared.SegmentTypeFloat
}

func (n *NumberSegment) Text() string {
	return fmt.Sprintf("%g", n.Value)
}

func (n *NumberSegment) Log() string {
	return fmt.Sprintf("%g", n.Value)
}

func (n *NumberSegment) Markdown() string {
	return fmt.Sprintf("%g", n.Value)
}

func (n *NumberSegment) Size() int {
	return int(unsafe.Sizeof(n.Value))
}

type FloatSegment struct {
	Value float64 `json:"value"`
}

func (f *FloatSegment) ToObject() any {
	return f.Value
}

func (f *FloatSegment) GetValue() any {
	return f.Value
}

func (f *FloatSegment) GetType() shared.SegmentType {
	return shared.SegmentTypeFloat
}

func (f *FloatSegment) Text() string {
	return fmt.Sprintf("%g", f.Value)
}

func (f *FloatSegment) Log() string {
	return fmt.Sprintf("%g", f.Value)
}

func (f *FloatSegment) Markdown() string {
	return fmt.Sprintf("%g", f.Value)
}

func (f *FloatSegment) Size() int {
	return int(unsafe.Sizeof(f.Value))
}

type IntegerSegment struct {
	Value int `json:"value"`
}

func (i *IntegerSegment) ToObject() any {
	return i.Value
}

func (i *IntegerSegment) GetValue() any {
	return i.Value
}

func (i *IntegerSegment) GetType() shared.SegmentType {
	return shared.SegmentTypeInteger
}

func (i *IntegerSegment) Text() string {
	return fmt.Sprintf("%d", i.Value)
}

func (i *IntegerSegment) Log() string {
	return fmt.Sprintf("%d", i.Value)
}

func (i *IntegerSegment) Markdown() string {
	return fmt.Sprintf("%d", i.Value)
}

func (i *IntegerSegment) Size() int {
	return int(unsafe.Sizeof(i.Value))
}

type BooleanSegment struct {
	Value bool `json:"value"`
}

func (b *BooleanSegment) ToObject() any {
	return b.Value
}

func (b *BooleanSegment) GetValue() any {
	return b.Value
}

func (b *BooleanSegment) GetType() shared.SegmentType {
	return shared.SegmentTypeBoolean
}

func (b *BooleanSegment) Text() string {
	return fmt.Sprintf("%t", b.Value)
}

func (b *BooleanSegment) Log() string {
	return fmt.Sprintf("%t", b.Value)
}

func (b *BooleanSegment) Markdown() string {
	return fmt.Sprintf("%t", b.Value)
}

func (b *BooleanSegment) Size() int {
	return int(unsafe.Sizeof(b.Value))
}
