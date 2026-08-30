package text2sql

import (
	"testing"

	"github.com/zgiai/zgi/api/pkg/sql_base"
)

func TestFuseRRFCombinesVectorAndBM25Ranks(t *testing.T) {
	t.Parallel()
	vector := []map[string]interface{}{{"asset_table_id": "orders"}, {"asset_table_id": "customers"}}
	bm25 := []map[string]interface{}{{"asset_table_id": "customers"}, {"asset_table_id": "products"}}
	got := fuseRRF(vector, bm25, 60)
	if len(got) != 3 || got[0].ID != "customers" {
		t.Fatalf("fuseRRF = %#v, want customers first and three candidates", got)
	}
}

func TestSchemaFingerprintIsStableAcrossInputOrder(t *testing.T) {
	t.Parallel()
	a := &sql_base.Table{Name: "orders", Columns: []sql_base.Column{{Name: "b", OrdinalPosition: 2}, {Name: "a", OrdinalPosition: 1}}}
	b := &sql_base.Table{Name: "orders", Columns: []sql_base.Column{{Name: "a", OrdinalPosition: 1}, {Name: "b", OrdinalPosition: 2}}}
	if schemaFingerprint(a) != schemaFingerprint(b) {
		t.Fatal("fingerprint changes with column input order")
	}
}
