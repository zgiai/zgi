package text2sql

import (
	"testing"

	"github.com/zgiai/zgi/api/pkg/sql_base"
)

func TestFuseRRFCombinesVectorAndBM25Ranks(t *testing.T) {
	t.Parallel()
	vector := []map[string]interface{}{{"asset_table_id": "orders"}, {"asset_table_id": "customers"}}
	bm25 := []map[string]interface{}{{"asset_table_id": "customers"}, {"asset_table_id": "products"}}
	got := fuseRRF(vector, bm25, 60, "")
	if len(got) != 3 || got[0].ID != "customers" {
		t.Fatalf("fuseRRF = %#v, want customers first and three candidates", got)
	}
}

func TestFuseRRFBreaksTiesWithLexicalEvidenceInsteadOfAssetID(t *testing.T) {
	t.Parallel()
	// A genuine RRF tie: each candidate holds the same rank in exactly one list,
	// so both would score identically. The asset ids are ordered so that the old
	// id-based tie-break picks the wrong one, while the lexical evidence points
	// at the right one.
	//
	// The description here stands in for indexed text that carries the user's
	// own domain wording. Until the index document includes data_source_tables
	// descriptions, this signal only sees table and column names.
	promotion := map[string]interface{}{"asset_table_id": "aaa-promotion", "table_name": "promotion_campaigns", "content": `{"name":"promotion_campaigns"}`}
	marketing := map[string]interface{}{"asset_table_id": "zzz-marketing", "table_name": "marketing_campaigns", "content": `{"name":"marketing_campaigns","description":"营销域campaigns业务表"}`}

	got := fuseRRF([]map[string]interface{}{promotion}, []map[string]interface{}{marketing}, 60, "营销业务的推广活动共记录了多少条？")
	if len(got) != 2 {
		t.Fatalf("fuseRRF = %#v, want two candidates", got)
	}
	if got[0].Score != got[1].Score {
		t.Fatalf("expected tied scores, got %#v", got)
	}
	if got[0].ID != "zzz-marketing" {
		t.Fatalf("tie-break picked %q, want the table with matching business wording", got[0].ID)
	}
}

func TestFuseRRFIsDeterministicWhenEvidenceAlsoTies(t *testing.T) {
	t.Parallel()
	a := map[string]interface{}{"asset_table_id": "a", "table_name": "alpha"}
	b := map[string]interface{}{"asset_table_id": "b", "table_name": "beta"}

	first := fuseRRF([]map[string]interface{}{a}, []map[string]interface{}{b}, 60, "unrelated question")
	second := fuseRRF([]map[string]interface{}{a}, []map[string]interface{}{b}, 60, "unrelated question")
	if first[0].Score != first[1].Score {
		t.Fatalf("expected tied scores, got %#v", first)
	}
	if first[0].ID != second[0].ID || first[1].ID != second[1].ID {
		t.Fatalf("fuseRRF is not deterministic: %#v vs %#v", first, second)
	}
}

func TestLexicalOverlapMatchesChineseWording(t *testing.T) {
	t.Parallel()
	matching := lexicalOverlap("营销业务的推广活动共记录了多少条？", `{"description":"营销域campaigns业务表"}`)
	unrelated := lexicalOverlap("营销业务的推广活动共记录了多少条？", `{"description":"售后域shipments业务表"}`)
	if matching <= unrelated {
		t.Fatalf("lexicalOverlap matching = %v, unrelated = %v; want matching greater", matching, unrelated)
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
