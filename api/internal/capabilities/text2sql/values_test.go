package text2sql

import (
	"context"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/pkg/sql_base"
	"github.com/zgiai/zgi/api/pkg/sql_base/audit"
	"gorm.io/datatypes"
)

type valueSamplingSQLBase struct {
	sql_base.SQLBase
	result *sql_base.QueryResult
	err    error
	query  string
	params []interface{}
}

func (f *valueSamplingSQLBase) ExecuteSQL(_ context.Context, query string, params []interface{}, _ *audit.Context) (*sql_base.QueryResult, error) {
	f.query = query
	f.params = params
	return f.result, f.err
}

func TestSampleColumnValuesFiltersNonCategoricalValues(t *testing.T) {
	client := &valueSamplingSQLBase{result: &sql_base.QueryResult{
		Columns: []string{"value", "column_name"},
		Rows: [][]any{
			{"urgent", "priority_level"},
			{"123", "priority_level"},
			{"true", "is_active"},
			{"Alice", "operator_name"},
			{"active", "campaign_status"},
		},
	}}
	table := &sql_base.Table{Schema: "public", Name: "campaigns", Columns: []sql_base.Column{
		{Name: "priority_level", DataType: "text", Enums: []string{"normal", "urgent"}},
		{Name: "is_active", DataType: "boolean"},
		{Name: "operator_name", DataType: "text"},
		{Name: "campaign_status", DataType: "text", Enums: []string{"active", "paused"}},
	}}
	got := SampleColumnValues(context.Background(), client, table)
	want := ColumnValues{"campaign_status": []string{"active"}, "priority_level": []string{"urgent"}}
	if len(got) != len(want) || len(got["priority_level"]) != 1 || got["priority_level"][0] != "urgent" || len(got["campaign_status"]) != 1 {
		t.Fatalf("SampleColumnValues() = %#v, want %#v", got, want)
	}
	if strings.Contains(client.query, "operator_name") || len(client.params) != 4 {
		t.Fatalf("sampling query = %q, params = %#v", client.query, client.params)
	}
}

func TestValueSamplesEncodeDecode(t *testing.T) {
	values := ColumnValues{"status": []string{"active", "pending"}}
	raw := EncodeValueSamples(values)
	if !ValueSamplesInitialized(raw) {
		t.Fatalf("encoded samples = %s, want initialized payload", raw)
	}
	decoded := DecodeValueSamples(raw)
	if len(decoded["status"]) != 2 || decoded["status"][1] != "pending" {
		t.Fatalf("decoded samples = %#v", decoded)
	}
	if got := DecodeValueSamples(datatypes.JSON([]byte(`{"v":99,"columns":{}}`))); len(got) != 0 {
		t.Fatalf("unknown version decoded as %#v, want empty", got)
	}
}
