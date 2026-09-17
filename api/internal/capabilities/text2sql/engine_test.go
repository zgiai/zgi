package text2sql

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/pkg/sql_base"
)

type fakeCatalogLoader struct {
	catalog Catalog
	err     error
}

func (f fakeCatalogLoader) Load(context.Context, CatalogRequest) (Catalog, error) {
	return f.catalog, f.err
}

func catalogWithTable(table *sql_base.Table) Catalog {
	return Catalog{Tables: []CatalogTable{{Ref: TableRef{AssetID: "table-1", PhysicalID: table.ID, Schema: table.Schema, Name: table.Name}, Table: table}}}
}

type fakeGenerator struct {
	responses []string
	calls     int
}

func (f *fakeGenerator) Generate(context.Context, GenerationRequest) (string, error) {
	if f.calls >= len(f.responses) {
		return "", errors.New("no response")
	}
	response := f.responses[f.calls]
	f.calls++
	return response, nil
}

func TestEngineManualModeLoadsFreshSchemaAndValidatesSQL(t *testing.T) {
	t.Parallel()
	generator := &fakeGenerator{responses: []string{`{"sql":"SELECT name FROM public.customers"}`}}
	table := &sql_base.Table{ID: 7, Schema: "public", Name: "customers", Columns: []sql_base.Column{{Name: "name", DataType: "text"}}}
	engine := NewEngine(fakeCatalogLoader{catalog: catalogWithTable(table)}, generator, nil)

	result, err := engine.Generate(context.Background(), Request{
		DataSourceID: "ds-1", Question: "客户名称", SelectionMode: SelectionModeManual,
		ManualTables:    []TableRef{{AssetID: "table-1", PhysicalID: 7}},
		GenerationModel: Model{Name: "model"}, RepairLimit: 1,
	})
	if err != nil {
		t.Fatalf("Generate error = %v", err)
	}
	if result.SQL != "SELECT name FROM public.customers" || generator.calls != 1 {
		t.Fatalf("SQL/calls = %q/%d", result.SQL, generator.calls)
	}
	if len(result.SchemaSelection.Selected) != 1 || result.SchemaSelection.Selected[0].PhysicalID != 7 || result.SchemaSelection.Selected[0].Name != "customers" {
		t.Fatalf("selected tables = %#v, want catalog-authoritative table", result.SchemaSelection.Selected)
	}
}

func TestParseAndValidateRejectsTrailingNonJSON(t *testing.T) {
	table := &sql_base.Table{ID: 7, Schema: "public", Name: "customers", Columns: []sql_base.Column{{Name: "name", DataType: "text"}}}
	_, err := parseAndValidate(`{"sql":"SELECT name FROM public.customers"} garbage`, catalogWithTable(table), nil)
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("parseAndValidate() error = %v, want ErrInvalidResponse", err)
	}
}

func TestRenderCatalogHonorsModeColumnLimitAndComments(t *testing.T) {
	comment := "customer name"
	catalog := catalogWithTable(&sql_base.Table{Schema: "public", Name: "customers", Columns: []sql_base.Column{
		{Name: "id", DataType: "uuid"},
		{Name: "name", DataType: "text", Comment: &comment},
	}})

	ddl := renderCatalog(catalog, SchemaRenderOptions{Mode: "ddl", MaxColumns: 2, IncludeComments: true})
	if ddl != "CREATE TABLE public.customers (\n  id uuid,\n  name text -- customer name\n);" {
		t.Fatalf("renderCatalog() = %q", ddl)
	}
	limited := renderCatalog(catalog, SchemaRenderOptions{Mode: "columns", MaxColumns: 1})
	if limited != "TABLE public.customers\n  id uuid\n  ... more columns omitted" {
		t.Fatalf("limited renderCatalog() = %q", limited)
	}
}

func TestRenderCatalogPutsBusinessDescriptionOnTableHeader(t *testing.T) {
	t.Parallel()

	// Two tables with an identical column layout, distinguished only by the
	// operator-maintained business description.
	catalog := Catalog{Tables: []CatalogTable{
		{
			Table:       &sql_base.Table{Schema: "public", Name: "zgi_base_tbl_marketing_campaigns_abc123", Columns: []sql_base.Column{{Name: "campaign_id", DataType: "bigint"}}},
			Description: "营销域campaigns业务表",
		},
		{
			Table:       &sql_base.Table{Schema: "public", Name: "zgi_base_tbl_promotion_campaigns_def456", Columns: []sql_base.Column{{Name: "campaign_id", DataType: "bigint"}}},
			Description: "促销域campaigns业务表",
		},
	}}

	rendered := renderCatalog(catalog, SchemaRenderOptions{Mode: "columns", IncludeComments: true})
	want := "TABLE public.zgi_base_tbl_marketing_campaigns_abc123\n" +
		"  -- 营销域campaigns业务表\n" +
		"  campaign_id bigint\n" +
		"TABLE public.zgi_base_tbl_promotion_campaigns_def456\n" +
		"  -- 促销域campaigns业务表\n" +
		"  campaign_id bigint"
	if rendered != want {
		t.Fatalf("renderCatalog() =\n%q\nwant\n%q", rendered, want)
	}
}

func TestRenderCatalogDDLKeepsDescriptionOutsideColumnBlock(t *testing.T) {
	t.Parallel()
	comment := "legacy physical comment"
	catalog := Catalog{Tables: []CatalogTable{{
		Table:       &sql_base.Table{Schema: "public", Name: "orders", Comment: &comment, Columns: []sql_base.Column{{Name: "id", DataType: "uuid"}, {Name: "amount", DataType: "numeric"}}},
		Description: "订单主表",
	}}}

	rendered := renderCatalog(catalog, SchemaRenderOptions{Mode: "ddl", IncludeComments: true})
	want := "CREATE TABLE public.orders (\n  -- 订单主表\n  id uuid,\n  amount numeric\n);"
	if rendered != want {
		t.Fatalf("renderCatalog() = %q, want %q", rendered, want)
	}
}

func TestRenderCatalogFallsBackToPhysicalComment(t *testing.T) {
	t.Parallel()
	comment := "Table created for organization org, data source ds"
	withComment := Catalog{Tables: []CatalogTable{{Table: &sql_base.Table{Schema: "public", Name: "customers", Comment: &comment, Columns: []sql_base.Column{{Name: "id", DataType: "uuid"}}}}}}
	if rendered := renderCatalog(withComment, SchemaRenderOptions{IncludeComments: true}); !strings.Contains(rendered, "-- "+comment) {
		t.Fatalf("renderCatalog() = %q, want physical comment fallback", rendered)
	}

	// A blank description must not emit an empty comment line.
	blank := Catalog{Tables: []CatalogTable{{Table: &sql_base.Table{Schema: "public", Name: "customers", Columns: []sql_base.Column{{Name: "id", DataType: "uuid"}}}}}}
	if rendered := renderCatalog(blank, SchemaRenderOptions{IncludeComments: true}); rendered != "TABLE public.customers\n  id uuid" {
		t.Fatalf("renderCatalog() = %q, want no comment line", rendered)
	}
}

func TestRenderCatalogOmitsDescriptionWhenCommentsDisabled(t *testing.T) {
	t.Parallel()
	catalog := Catalog{Tables: []CatalogTable{{
		Table:       &sql_base.Table{Schema: "public", Name: "customers", Columns: []sql_base.Column{{Name: "id", DataType: "uuid"}}},
		Description: "客户主数据",
	}}}
	if rendered := renderCatalog(catalog, SchemaRenderOptions{}); strings.Contains(rendered, "客户主数据") {
		t.Fatalf("renderCatalog() = %q, want description suppressed", rendered)
	}
}

func TestEngineRejectsMarkdownAndRepairsOnce(t *testing.T) {
	t.Parallel()
	generator := &fakeGenerator{responses: []string{"```json\n{\"sql\":\"SELECT name FROM customers\"}\n```", `{"sql":"SELECT name FROM customers"}`}}
	table := &sql_base.Table{ID: 7, Name: "customers", Columns: []sql_base.Column{{Name: "name"}}}
	engine := NewEngine(fakeCatalogLoader{catalog: catalogWithTable(table)}, generator, nil)

	result, err := engine.Generate(context.Background(), Request{DataSourceID: "ds", Question: "q", SelectionMode: SelectionModeManual, ManualTables: []TableRef{{PhysicalID: 7}}, GenerationModel: Model{Name: "m"}, RepairLimit: 1})
	if err != nil {
		t.Fatalf("Generate error = %v", err)
	}
	if result.RepairCount != 1 || generator.calls != 2 {
		t.Fatalf("RepairCount/calls = %d/%d, want 1/2", result.RepairCount, generator.calls)
	}
}

func TestEngineRejectsUnknownColumn(t *testing.T) {
	t.Parallel()
	generator := &fakeGenerator{responses: []string{`{"sql":"SELECT invented FROM customers"}`}}
	table := &sql_base.Table{ID: 7, Name: "customers", Columns: []sql_base.Column{{Name: "name"}}}
	engine := NewEngine(fakeCatalogLoader{catalog: catalogWithTable(table)}, generator, nil)

	_, err := engine.Generate(context.Background(), Request{DataSourceID: "ds", Question: "q", SelectionMode: SelectionModeManual, ManualTables: []TableRef{{PhysicalID: 7}}, GenerationModel: Model{Name: "m"}})
	if !errors.Is(err, ErrColumnNotFound) {
		t.Fatalf("Generate error = %v, want ErrColumnNotFound", err)
	}
}

func TestParseAndValidateRejectsColumnOwnedByAnotherQualifiedTable(t *testing.T) {
	t.Parallel()
	catalog := Catalog{Tables: []CatalogTable{
		{Table: &sql_base.Table{Schema: "public", Name: "users", Columns: []sql_base.Column{{Name: "id"}}}},
		{Table: &sql_base.Table{Schema: "public", Name: "orders", Columns: []sql_base.Column{{Name: "id"}, {Name: "total"}}}},
	}}
	query := `SELECT u.total FROM users AS u JOIN orders AS o ON u.id = o.id`

	_, err := parseAndValidate(`{"sql":`+strconv.Quote(query)+`}`, catalog, nil)
	if !errors.Is(err, ErrColumnNotFound) {
		t.Fatalf("parseAndValidate() error = %v, want ErrColumnNotFound", err)
	}
}

func TestParseAndValidateAllowsCurrentSelectAliasInOrderByAndGroupBy(t *testing.T) {
	t.Parallel()
	catalog := Catalog{Tables: []CatalogTable{{Table: &sql_base.Table{
		Name:    "order_items",
		Columns: []sql_base.Column{{Name: "product_id"}, {Name: "quantity"}, {Name: "unit_price"}},
	}}}}

	queries := []string{
		`SELECT product_id, SUM(quantity * unit_price) AS total_sales_amount FROM order_items GROUP BY product_id ORDER BY total_sales_amount DESC`,
		`SELECT product_id AS product, COUNT(*) FROM order_items GROUP BY product`,
	}
	for _, query := range queries {
		if _, err := parseAndValidate(`{"sql":`+strconv.Quote(query)+`}`, catalog, nil); err != nil {
			t.Fatalf("parseAndValidate(%q) error = %v", query, err)
		}
	}
}

func TestParseAndValidateDoesNotExposeCurrentSelectAliasToEarlierClauses(t *testing.T) {
	t.Parallel()
	catalog := Catalog{Tables: []CatalogTable{{Table: &sql_base.Table{
		Name:    "order_items",
		Columns: []sql_base.Column{{Name: "quantity"}, {Name: "unit_price"}},
	}}}}

	queries := []string{
		`SELECT quantity * unit_price AS amount FROM order_items WHERE amount > 100`,
		`SELECT SUM(quantity * unit_price) AS amount FROM order_items HAVING amount > 100`,
		`SELECT a.quantity * a.unit_price AS amount FROM order_items AS a JOIN order_items AS b ON amount > b.quantity`,
	}
	for _, query := range queries {
		_, err := parseAndValidate(`{"sql":`+strconv.Quote(query)+`}`, catalog, nil)
		if !errors.Is(err, ErrColumnNotFound) {
			t.Fatalf("parseAndValidate(%q) error = %v, want ErrColumnNotFound", query, err)
		}
	}
}

func TestParseAndValidateDoesNotLeakAliasAcrossSubqueryScopes(t *testing.T) {
	t.Parallel()
	catalog := Catalog{Tables: []CatalogTable{{Table: &sql_base.Table{
		Name:    "order_items",
		Columns: []sql_base.Column{{Name: "quantity"}, {Name: "unit_price"}},
	}}}}
	query := `SELECT invented FROM order_items WHERE EXISTS (SELECT quantity AS invented FROM order_items ORDER BY invented)`

	_, err := parseAndValidate(`{"sql":`+strconv.Quote(query)+`}`, catalog, nil)
	if !errors.Is(err, ErrColumnNotFound) {
		t.Fatalf("parseAndValidate() error = %v, want ErrColumnNotFound", err)
	}
}

func TestParseAndValidateAllowsColumnsProducedByCTE(t *testing.T) {
	t.Parallel()
	catalog := Catalog{Tables: []CatalogTable{{Table: &sql_base.Table{
		Name:    "orders",
		Columns: []sql_base.Column{{Name: "order_date"}, {Name: "total_amount"}},
	}}}}
	query := `WITH monthly AS (
		SELECT DATE_TRUNC('month', order_date) AS month_start, SUM(total_amount) AS revenue
		FROM orders
		GROUP BY DATE_TRUNC('month', order_date)
	) SELECT month_start, revenue FROM monthly ORDER BY month_start`

	if _, err := parseAndValidate(`{"sql":`+strconv.Quote(query)+`}`, catalog, nil); err != nil {
		t.Fatalf("parseAndValidate() error = %v", err)
	}
}

func TestParseAndValidateAllowsWindowAliasProducedByCTE(t *testing.T) {
	t.Parallel()
	catalog := Catalog{Tables: []CatalogTable{{Table: &sql_base.Table{
		Name:    "orders",
		Columns: []sql_base.Column{{Name: "customer_id"}, {Name: "order_date"}},
	}}}}
	query := `WITH ranked AS (
		SELECT customer_id, ROW_NUMBER() OVER (PARTITION BY customer_id ORDER BY order_date) AS rn
		FROM orders
	) SELECT customer_id FROM ranked WHERE rn = 1`

	if _, err := parseAndValidate(`{"sql":`+strconv.Quote(query)+`}`, catalog, nil); err != nil {
		t.Fatalf("parseAndValidate() error = %v", err)
	}
}

func TestParseAndValidateAllowsColumnsAcrossChainedCTEs(t *testing.T) {
	t.Parallel()
	catalog := Catalog{Tables: []CatalogTable{{Table: &sql_base.Table{
		Name:    "orders",
		Columns: []sql_base.Column{{Name: "customer_id"}, {Name: "order_date"}},
	}}}}
	query := `WITH ranked AS (
		SELECT customer_id, ROW_NUMBER() OVER (PARTITION BY customer_id ORDER BY order_date) AS rn
		FROM orders
	), first_orders AS (
		SELECT customer_id, rn FROM ranked WHERE rn = 1
	) SELECT customer_id FROM first_orders`

	if _, err := parseAndValidate(`{"sql":`+strconv.Quote(query)+`}`, catalog, nil); err != nil {
		t.Fatalf("parseAndValidate() error = %v", err)
	}
}

func TestParseAndValidateAllowsColumnsProducedByDerivedTable(t *testing.T) {
	t.Parallel()
	catalog := Catalog{Tables: []CatalogTable{{Table: &sql_base.Table{
		Name:    "orders",
		Columns: []sql_base.Column{{Name: "customer_id"}},
	}}}}
	query := `SELECT totals.customer_id, totals.order_count
		FROM (SELECT customer_id, COUNT(*) AS order_count FROM orders GROUP BY customer_id) AS totals
		WHERE totals.order_count > 1`

	if _, err := parseAndValidate(`{"sql":`+strconv.Quote(query)+`}`, catalog, nil); err != nil {
		t.Fatalf("parseAndValidate() error = %v", err)
	}
}
