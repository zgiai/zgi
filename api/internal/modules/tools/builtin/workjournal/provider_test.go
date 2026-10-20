package workjournal

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	journal "github.com/zgiai/zgi/api/internal/capabilities/workjournal"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type sessionResolver struct {
	session Session
	got     InvocationIdentity
}

func (r *sessionResolver) ResolveWorkJournal(_ context.Context, i InvocationIdentity) (Session, error) {
	r.got = i
	return r.session, nil
}

type gateway struct{ calls int }

func (g *gateway) Execute(_ context.Context, op journal.Operation) (journal.DispatchResult, error) {
	g.calls++
	return journal.DispatchResult{State: journal.StateSucceeded, Receipt: datatypes.JSON(`{"message_id":"real-provider-id"}`)}, nil
}
func (g *gateway) Recover(context.Context, journal.Operation) (journal.DispatchResult, error) {
	panic("unexpected recover")
}
func TestOneProviderSupportsSocialAndCustomerSupportWithoutSocialConfiguration(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	if e = db.AutoMigrate(&journal.Record{}, &journal.RecordVersion{}, &journal.Operation{}); e != nil {
		t.Fatal(e)
	}
	service, e := journal.New(db, nil)
	if e != nil {
		t.Fatal(e)
	}
	scope := journal.Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), OwnerID: uuid.New(), EmployeeID: uuid.New()}
	g := &gateway{}
	r := &sessionResolver{session: Session{Scope: scope, EmployeeVersionID: uuid.New(), Gateway: g}}
	p := NewProvider(func() (*journal.Service, error) { return service, nil }, r)
	invoke := func(name string, input any) map[string]any {
		t.Helper()
		tool, e := p.GetTool(name)
		if e != nil {
			t.Fatal(e)
		}
		tool = tool.ForkToolRuntime(&tools.ToolRuntime{TenantID: scope.OrganizationID.String(), InvokeFrom: tools.ToolInvokeFromDigitalHuman})
		b, _ := json.Marshal(input)
		c, a, m := uuid.NewString(), scope.EmployeeID.String(), uuid.NewString()
		result, e := tool.Invoke(t.Context(), scope.OwnerID.String(), map[string]any{"input": string(b)}, &c, &a, &m)
		if e != nil || len(result) != 1 {
			t.Fatal(result, e)
		}
		data, e := json.Marshal(result[0].Data)
		if e != nil {
			t.Fatal(e)
		}
		var out map[string]any
		if e = json.Unmarshal(data, &out); e != nil {
			t.Fatal(string(data), e)
		}
		return out
	}
	for _, kind := range []string{"social_draft", "support_ticket"} {
		invoke("save_record", map[string]any{"kind": kind, "key": "case-1", "state": "waiting_for_review", "content": map[string]any{"text": "scenario-specific"}, "sources": []string{"https://example.com/source"}, "expected_revision": 0})
		rows, e := service.ListRecords(t.Context(), scope, kind, uuid.Nil, 20)
		if e != nil || len(rows) != 1 {
			t.Fatal(rows, e)
		}
	}
	for _, connector := range []string{"x-social", "gmail"} {
		operation := connector + ".message.send"
		arguments := map[string]any{"to": []string{"customer@example.com"}, "subject": "Support reply", "text": "approved response"}
		if connector == "x-social" {
			operation = "x-social.post.create"
			arguments = map[string]any{"account_id": "authorized-account", "text": "approved response"}
		}
		out := invoke("prepare_operation", map[string]any{"work_key": connector + "-source-1", "connector_id": connector, "connection_id": uuid.NewString(), "operation_id": operation, "arguments": arguments})
		op := out["operation"].(map[string]any)
		id := op["id"].(string)
		invoke("dispatch_operation", map[string]any{"operation_id": id})
		invoke("dispatch_operation", map[string]any{"operation_id": id})
	}
	if g.calls != 2 {
		t.Fatalf("duplicate dispatch calls=%d", g.calls)
	}
	tool, _ := p.GetTool("prepare_operation")
	tool = tool.ForkToolRuntime(&tools.ToolRuntime{})
	for _, raw := range []string{`{"owner_id":"forged"}`, `{"employee_version_id":"forged"}`, `{} {}`, `{"receipt":{"id":"forged"}}`} {
		if _, e = tool.Invoke(t.Context(), scope.OwnerID.String(), map[string]any{"input": raw}, nil, nil, nil); e == nil {
			t.Fatalf("accepted authority input %s", raw)
		}
	}
}

func TestPagesRetainCursorWhenRecordPayloadExceedsToolBudget(t *testing.T) {
	rows := []journal.Record{{ID: uuid.New(), Content: datatypes.JSON(`"` + strings.Repeat("a", 48000) + `"`)}, {ID: uuid.New(), Content: datatypes.JSON(`"` + strings.Repeat("b", 48000) + `"`)}, {ID: uuid.New(), Content: datatypes.JSON(`"` + strings.Repeat("c", 48000) + `"`)}}
	page := boundedPage(rows, 3, "next_after_id", func(r journal.Record) any { return r.ID })
	kept := page["items"].([]journal.Record)
	if len(kept) != 2 || page["next_after_id"] != kept[1].ID {
		t.Fatal("page lost resume cursor", len(kept), page["next_after_id"])
	}
	b, err := json.Marshal(page)
	if err != nil || len(b) > 120000 {
		t.Fatal(len(b), err)
	}
}
