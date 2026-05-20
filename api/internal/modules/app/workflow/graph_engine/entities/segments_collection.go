package entities

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

type ObjectSegment struct {
	Value map[string]interface{} `json:"value"`
}

func (o *ObjectSegment) ToObject() any {
	return o.Value
}

func (o *ObjectSegment) GetValue() any {
	return o.Value
}

func (o *ObjectSegment) GetType() shared.SegmentType {
	return shared.SegmentTypeObject
}

func (o *ObjectSegment) Text() string {
	data, err := json.Marshal(o.Value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func (o *ObjectSegment) Log() string {
	data, err := json.MarshalIndent(o.Value, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

func (o *ObjectSegment) Markdown() string {
	data, err := json.MarshalIndent(o.Value, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

func (o *ObjectSegment) Size() int {
	data, err := json.Marshal(o.Value)
	if err != nil {
		return 0
	}
	return len(data)
}

type NoneSegment struct{}

func (n *NoneSegment) ToObject() any {
	return nil
}

func (n *NoneSegment) GetValue() any {
	return nil
}

func (n *NoneSegment) GetType() shared.SegmentType {
	return shared.SegmentTypeNone
}

func (n *NoneSegment) Text() string {
	return ""
}

func (n *NoneSegment) Log() string {
	return ""
}

func (n *NoneSegment) Markdown() string {
	return ""
}

func (n *NoneSegment) Size() int {
	return 0
}

type SegmentGroup struct {
	Value []Segment `json:"value"`
}

func (sg *SegmentGroup) ToObject() any {
	result := make([]any, len(sg.Value))
	for i, segment := range sg.Value {
		result[i] = segment.ToObject()
	}
	return result
}

func (sg *SegmentGroup) GetValue() any {
	return sg.Value
}

func (sg *SegmentGroup) GetType() shared.SegmentType {
	return shared.SegmentTypeGroup
}

func (sg *SegmentGroup) Text() string {
	var texts []string
	for _, segment := range sg.Value {
		texts = append(texts, segment.Text())
	}
	return strings.Join(texts, "")
}

func (sg *SegmentGroup) Log() string {
	var logs []string
	for _, segment := range sg.Value {
		logs = append(logs, segment.Log())
	}
	return strings.Join(logs, "")
}

func (sg *SegmentGroup) Markdown() string {
	var markdowns []string
	for _, segment := range sg.Value {
		markdowns = append(markdowns, segment.Markdown())
	}
	return strings.Join(markdowns, "")
}

func (sg *SegmentGroup) Size() int {
	totalSize := 0
	for _, segment := range sg.Value {
		totalSize += segment.Size()
	}
	return totalSize
}

type ArrayAnySegment struct {
	Value []any `json:"value"`
}

func (a *ArrayAnySegment) ToObject() any {
	return a.Value
}

func (a *ArrayAnySegment) GetValue() any {
	return a.Value
}

func (a *ArrayAnySegment) GetType() shared.SegmentType {
	return shared.SegmentTypeArrayAny
}

func (a *ArrayAnySegment) Text() string {
	data, err := json.Marshal(a.Value)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func (a *ArrayAnySegment) Log() string {
	data, err := json.MarshalIndent(a.Value, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(data)
}

func (a *ArrayAnySegment) Markdown() string {
	data, err := json.MarshalIndent(a.Value, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(data)
}

func (a *ArrayAnySegment) Size() int {
	return len(a.Value)
}

type ArrayStringSegment struct {
	Value []string `json:"value"`
}

func (a *ArrayStringSegment) ToObject() any {
	return a.Value
}

func (a *ArrayStringSegment) GetValue() any {
	return a.Value
}

func (a *ArrayStringSegment) GetType() shared.SegmentType {
	return shared.SegmentTypeArrayString
}

func (a *ArrayStringSegment) Text() string {
	return strings.Join(a.Value, ", ")
}

func (a *ArrayStringSegment) Log() string {
	data, err := json.MarshalIndent(a.Value, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(data)
}

func (a *ArrayStringSegment) Markdown() string {
	data, err := json.MarshalIndent(a.Value, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(data)
}

func (a *ArrayStringSegment) Size() int {
	return len(a.Value)
}

type ArrayNumberSegment struct {
	Value []float64 `json:"value"`
}

func (a *ArrayNumberSegment) ToObject() any {
	return a.Value
}

func (a *ArrayNumberSegment) GetValue() any {
	return a.Value
}

func (a *ArrayNumberSegment) GetType() shared.SegmentType {
	return shared.SegmentTypeArrayNumber
}

func (a *ArrayNumberSegment) Text() string {
	data, err := json.Marshal(a.Value)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func (a *ArrayNumberSegment) Log() string {
	data, err := json.MarshalIndent(a.Value, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(data)
}

func (a *ArrayNumberSegment) Markdown() string {
	data, err := json.MarshalIndent(a.Value, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(data)
}

func (a *ArrayNumberSegment) Size() int {
	return len(a.Value)
}

type ArrayObjectSegment struct {
	Value []map[string]interface{} `json:"value"`
}

func (a *ArrayObjectSegment) ToObject() any {
	return a.Value
}

func (a *ArrayObjectSegment) GetValue() any {
	return a.Value
}

func (a *ArrayObjectSegment) GetType() shared.SegmentType {
	return shared.SegmentTypeArrayObject
}

func (a *ArrayObjectSegment) Text() string {
	data, err := json.Marshal(a.Value)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func (a *ArrayObjectSegment) Log() string {
	data, err := json.MarshalIndent(a.Value, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(data)
}

func (a *ArrayObjectSegment) Markdown() string {
	data, err := json.MarshalIndent(a.Value, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(data)
}

func (a *ArrayObjectSegment) Size() int {
	return len(a.Value)
}

type ArrayFileSegment struct {
	Value []*File `json:"value"`
}

func (a *ArrayFileSegment) ToObject() any {
	return a.Value
}

func (a *ArrayFileSegment) GetValue() any {
	return a.Value
}

func (a *ArrayFileSegment) GetType() shared.SegmentType {
	return shared.SegmentTypeArrayFile
}

func (a *ArrayFileSegment) Text() string {
	return ""
}

func (a *ArrayFileSegment) Log() string {
	return ""
}

func (a *ArrayFileSegment) Markdown() string {
	var markdowns []string
	for _, file := range a.Value {
		if file != nil {
			if strings.HasPrefix(file.MimeType, "image/") {
				markdowns = append(markdowns, fmt.Sprintf("![%s](%s)", file.Filename, file.RemoteURL))
			} else {
				markdowns = append(markdowns, fmt.Sprintf("[%s](%s)", file.Filename, file.RemoteURL))
			}
		}
	}
	return strings.Join(markdowns, "\n")
}

func (a *ArrayFileSegment) Size() int {
	return len(a.Value)
}

type ArrayBooleanSegment struct {
	Value []bool `json:"value"`
}

func (a *ArrayBooleanSegment) ToObject() any {
	return a.Value
}

func (a *ArrayBooleanSegment) GetValue() any {
	return a.Value
}

func (a *ArrayBooleanSegment) GetType() shared.SegmentType {
	return shared.SegmentTypeArrayBoolean
}

func (a *ArrayBooleanSegment) Text() string {
	data, err := json.Marshal(a.Value)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func (a *ArrayBooleanSegment) Log() string {
	data, err := json.MarshalIndent(a.Value, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(data)
}

func (a *ArrayBooleanSegment) Markdown() string {
	data, err := json.MarshalIndent(a.Value, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(data)
}

func (a *ArrayBooleanSegment) Size() int {
	return len(a.Value)
}
