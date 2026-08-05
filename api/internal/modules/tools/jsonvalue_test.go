package tools

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeJSONValueCanonicalizesNestedTypedCollections(t *testing.T) {
	input := map[string]interface{}{
		"folders": []map[string]interface{}{
			{"name": "INBOX", "attributes": []string{"\\HasNoChildren"}},
		},
		"department_ids": []int{1, 2},
	}

	normalized, err := NormalizeJSONValue(input)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]interface{}{
		"folders": []interface{}{
			map[string]interface{}{"name": "INBOX", "attributes": []interface{}{`\HasNoChildren`}},
		},
		"department_ids": []interface{}{float64(1), float64(2)},
	}
	if !reflect.DeepEqual(normalized, want) {
		t.Fatalf("normalized = %#v, want %#v", normalized, want)
	}
}

func TestNormalizeJSONValueRejectsNonJSONValues(t *testing.T) {
	for name, input := range map[string]interface{}{
		"channel": map[string]interface{}{"value": make(chan int)},
		"nan":     map[string]interface{}{"value": math.NaN()},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NormalizeJSONValue(input); err == nil {
				t.Fatal("expected normalization error")
			}
		})
	}
}

func TestNormalizeJSONValueRejectsOversizedValue(t *testing.T) {
	_, err := NormalizeJSONValue(map[string]interface{}{"value": strings.Repeat("x", maxNormalizedJSONValueBytes)})
	if err == nil {
		t.Fatal("expected oversized normalization error")
	}
}
