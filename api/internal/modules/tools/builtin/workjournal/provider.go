// Package workjournal exposes durable business records and a connector-neutral
// external-operation journal. Business procedures remain employee instructions.
package workjournal

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/google/uuid"
	journal "github.com/zgiai/zgi/api/internal/capabilities/workjournal"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
	"gorm.io/datatypes"
)

const ProviderID = "work_journal"

type InvocationIdentity struct {
	Runtime                                        *tools.ToolRuntime
	ActorID, ConversationID, EmployeeID, MessageID string
}
type Session struct {
	Scope             journal.Scope
	EmployeeVersionID uuid.UUID
	Gateway           journal.Gateway
}
type ScopeResolver interface {
	ResolveWorkJournal(context.Context, InvocationIdentity) (Session, error)
}
type ServiceProvider func() (*journal.Service, error)
type Provider struct{ *builtin.BuiltinProvider }

func NewProvider(service ServiceProvider, resolver ScopeResolver) *Provider {
	p := &Provider{builtin.NewBuiltinProvider(tools.ToolProviderIdentity{Name: ProviderID, Author: "System", Label: tools.I18nText{"en_US": "Durable Work", "zh_Hans": "持久化工作记录"}, Description: tools.I18nText{"en_US": "Scenario-neutral records and authorized connector operation tracking.", "zh_Hans": "跨场景工作记录与已授权连接器操作状态。"}, Icon: "clipboard-list", Tags: []string{"operations"}})}
	descriptions := map[string]string{
		"save_record":        "Create or CAS-update a business record with arbitrary kind,key,state,content(JSON),sources(JSON array),expected_revision(0 creates). Business state is scenario-defined and never proves a send. Retains immutable revision history.",
		"read_record":        "Read a scoped business record by record_id OR kind+key, never both.",
		"list_records":       "Page business records by kind(optional),after_id(optional UUID),limit(1..100,default20).",
		"record_history":     "Read immutable record revisions: record_id,after_revision(optional integer),limit(1..100,default20).",
		"prepare_operation":  "Freeze one native connector request without sending. Input: work_key(stable source/business-action key reused across runs),connector_id,connection_id(UUID),operation_id(exact native name),arguments(JSON object matching the exposed connector schema). No credentials or authorization fields. Reusing a key with changed arguments conflicts. Inspect native connector schemas first.",
		"dispatch_operation": "Execute one pending operation through live connector authorization. Input: operation_id(journal UUID). Repeated executing/unknown/terminal operations never resend. Read returned state and receipt; succeeded proves connector execution only, not recipient reading or completed business workflow.",
		"read_operation":     "Read durable state/receipt by operation_id(journal UUID) OR work_key, never both.",
		"list_operations":    "Page journal operations: state(optional pending/executing/succeeded/failed/unknown),after_id(optional UUID),limit(1..100,default20).",
		"recover_operation":  "Recover an executing/unknown operation using server connector execution evidence, without sending again. Input: operation_id(journal UUID). Cannot accept model-authored receipts or reset state. Never replace an uncertain send with another work key.",
	}
	for name, description := range descriptions {
		entity := tools.ToolEntity{Identity: tools.ToolIdentity{Name: name, Provider: ProviderID, Author: "System", Label: tools.I18nText{"en_US": name}}, Description: tools.ToolDescription{LLM: description, Human: tools.I18nText{"en_US": description}}, Parameters: []tools.ToolParameter{{Name: "input", Label: tools.I18nText{"en_US": "JSON input", "zh_Hans": "操作参数"}, LLMDescription: description + " Supply one JSON object string. Owner/workspace/employee/version/receipt authority fields are forbidden.", Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM, Required: true, SupportVariable: true}}, OutputType: "json"}
		p.RegisterTool(&journalTool{BuiltinTool: builtin.NewBuiltinTool(entity, ""), action: name, service: service, resolver: resolver})
	}
	return p
}

type journalTool struct {
	*builtin.BuiltinTool
	action   string
	service  ServiceProvider
	resolver ScopeResolver
}

func (t *journalTool) ForkToolRuntime(r *tools.ToolRuntime) tools.Tool {
	return &journalTool{BuiltinTool: t.BuiltinTool.ForkToolRuntime(r), action: t.action, service: t.service, resolver: t.resolver}
}
func invalid() error { return core.NewError(core.ErrorCodeInvalidInput, false, nil) }
func decode(raw string, target any) error {
	d := json.NewDecoder(bytes.NewBufferString(raw))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		return invalid()
	}
	return nil
}
func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
func (t *journalTool) Invoke(ctx context.Context, user string, params map[string]interface{}, conversation, employee, message *string) ([]tools.ToolInvokeMessage, error) {
	raw, ok := params["input"].(string)
	if !ok || len(params) != 1 || len(raw) > 60000 || t.Runtime() == nil || t.service == nil || t.resolver == nil {
		return nil, invalid()
	}
	session, err := t.resolver.ResolveWorkJournal(ctx, InvocationIdentity{Runtime: t.Runtime(), ActorID: user, ConversationID: str(conversation), EmployeeID: str(employee), MessageID: str(message)})
	if err != nil {
		return nil, err
	}
	if session.EmployeeVersionID == uuid.Nil {
		return nil, invalid()
	}
	service, err := t.service()
	if err != nil {
		return nil, err
	}
	if service == nil {
		return nil, invalid()
	}
	out, err := t.execute(ctx, service, session, raw)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(out)
	if err != nil || len(encoded) > 120000 {
		return nil, invalid()
	}
	var envelope map[string]interface{}
	d := json.NewDecoder(bytes.NewReader(encoded))
	d.UseNumber()
	if d.Decode(&envelope) != nil {
		return nil, invalid()
	}
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(envelope)}, nil
}
func (t *journalTool) execute(ctx context.Context, s *journal.Service, session Session, raw string) (any, error) {
	scope := session.Scope
	switch t.action {
	case "save_record":
		var r struct {
			Kind             string         `json:"kind"`
			Key              string         `json:"key"`
			State            string         `json:"state"`
			Content          datatypes.JSON `json:"content"`
			Sources          datatypes.JSON `json:"sources"`
			ExpectedRevision uint64         `json:"expected_revision"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		row, err := s.SaveRecord(ctx, scope, journal.SaveRecordRequest{Kind: r.Kind, Key: r.Key, State: r.State, Content: r.Content, Sources: r.Sources, ExpectedRevision: r.ExpectedRevision})
		return map[string]any{"record": row}, err
	case "read_record":
		var r struct {
			ID   uuid.UUID `json:"record_id"`
			Kind string    `json:"kind"`
			Key  string    `json:"key"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		if r.ID != uuid.Nil {
			if r.Kind != "" || r.Key != "" {
				return nil, invalid()
			}
			row, err := s.GetRecord(ctx, scope, r.ID)
			return map[string]any{"record": row}, err
		}
		row, err := s.GetRecordByKey(ctx, scope, r.Kind, r.Key)
		return map[string]any{"record": row}, err
	case "list_records", "list_operations":
		var r struct {
			Kind  string    `json:"kind,omitempty"`
			State string    `json:"state,omitempty"`
			After uuid.UUID `json:"after_id,omitempty"`
			Limit int       `json:"limit,omitempty"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		if r.Limit == 0 {
			r.Limit = 20
		}
		if r.Limit < 1 || r.Limit > 100 {
			return nil, invalid()
		}
		if t.action == "list_records" {
			if r.State != "" {
				return nil, invalid()
			}
			rows, err := s.ListRecords(ctx, scope, r.Kind, r.After, r.Limit)
			return boundedPage(rows, r.Limit, "next_after_id", func(row journal.Record) any { return row.ID }), err
		}
		if r.Kind != "" {
			return nil, invalid()
		}
		rows, err := s.ListOperations(ctx, scope, r.State, r.After, r.Limit)
		return boundedPage(rows, r.Limit, "next_after_id", func(row journal.Operation) any { return row.ID }), err
	case "record_history":
		var r struct {
			ID    uuid.UUID `json:"record_id"`
			After uint64    `json:"after_revision,omitempty"`
			Limit int       `json:"limit,omitempty"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		if r.Limit == 0 {
			r.Limit = 20
		}
		if r.Limit < 1 || r.Limit > 100 {
			return nil, invalid()
		}
		rows, err := s.ListRecordVersions(ctx, scope, r.ID, r.After, r.Limit)
		return boundedPage(rows, r.Limit, "next_after_revision", func(row journal.RecordVersion) any { return row.Revision }), err
	case "prepare_operation":
		var r struct {
			Key        string         `json:"work_key"`
			Connector  string         `json:"connector_id"`
			Connection uuid.UUID      `json:"connection_id"`
			Operation  string         `json:"operation_id"`
			Arguments  datatypes.JSON `json:"arguments"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		row, err := s.Prepare(ctx, scope, journal.PrepareRequest{WorkKey: r.Key, EmployeeVersionID: session.EmployeeVersionID, ConnectorID: r.Connector, ConnectionID: r.Connection, OperationID: r.Operation, Input: r.Arguments})
		return map[string]any{"operation": row}, err
	case "read_operation":
		var r struct {
			ID  uuid.UUID `json:"operation_id"`
			Key string    `json:"work_key"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		if r.ID != uuid.Nil {
			if r.Key != "" {
				return nil, invalid()
			}
			row, err := s.GetOperation(ctx, scope, r.ID)
			return map[string]any{"operation": row}, err
		}
		row, err := s.GetOperationByKey(ctx, scope, r.Key)
		return map[string]any{"operation": row}, err
	case "dispatch_operation", "recover_operation":
		var r struct {
			ID uuid.UUID `json:"operation_id"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		if session.Gateway == nil {
			return nil, invalid()
		}
		var row journal.Operation
		var err error
		if t.action == "dispatch_operation" {
			row, err = s.Dispatch(ctx, scope, r.ID, session.Gateway)
		} else {
			row, err = s.Recover(ctx, scope, r.ID, session.Gateway)
		}
		return map[string]any{"operation": row}, err
	}
	return nil, invalid()
}

// Pages retain an explicit cursor when large business records exhaust the tool
// output budget before the requested row count. Nothing is silently skipped.
func boundedPage[T any](rows []T, requested int, cursorKey string, cursor func(T) any) map[string]any {
	kept := make([]T, 0, len(rows))
	size := 128
	for _, row := range rows {
		encoded, err := json.Marshal(row)
		if err != nil || size+len(encoded) > 110000 {
			break
		}
		kept = append(kept, row)
		size += len(encoded) + 1
	}
	var next any
	if len(kept) > 0 && (len(kept) < len(rows) || len(rows) == requested) {
		next = cursor(kept[len(kept)-1])
	}
	return map[string]any{"items": kept, cursorKey: next}
}
