package socialoperations

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/socialmcp"
	"github.com/zgiai/zgi/api/internal/modules/socialops"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

const ProviderID = "social_operations"

type InvocationIdentity struct {
	Runtime                                        *tools.ToolRuntime
	ActorID, ConversationID, EmployeeID, MessageID string
}
type ScopeResolver interface {
	ResolveSocialOperations(context.Context, InvocationIdentity) (socialops.Scope, socialops.PublicationGateway, error)
}

type ImageImporter interface {
	ImportSocialImage(context.Context, InvocationIdentity, uuid.UUID) ([]byte, error)
}
type ImageInspector interface {
	InspectSocialImage(context.Context, InvocationIdentity, uuid.UUID, uuid.UUID) (map[string]any, error)
}
type ServiceProvider func() (*socialops.Service, error)
type Provider struct{ *builtin.BuiltinProvider }

func NewProvider(service ServiceProvider, resolver ScopeResolver) *Provider {
	p := &Provider{builtin.NewBuiltinProvider(tools.ToolProviderIdentity{Name: ProviderID, Author: "System", Label: tools.I18nText{"en_US": "Social Operations", "zh_Hans": "社交运营执行记录"}, Description: tools.I18nText{"en_US": "Durable, scoped employee SOP records and verified publishing."}, Icon: "notebook", Tags: []string{"operations"}})}
	for name, description := range map[string]string{
		"inspect_image":         "Inspect actual scoped image pixels with the configured organization vision model before image publication. Input: artifact_id,logo_artifact_id(optional approved logo; required when more than three brand references). Returns a persisted review artifact with findings; failed inspection does not permit publication. No arbitrary URLs or paths.",
		"compose_image":         "Compose a consistent 1600x900 publication image using exact approved logo pixels, literal headline/body and optional scoped background image. Input: logo_artifact_id (must be in canonical brand refs),headline,body,background_artifact_id(optional). Rejects text overflow and unsupported glyphs; never regenerates a logo. Use returned artifact in publication images.",
		"list_publications":     "Read verified publication history together with preserved metric samples for weekly review; missing metrics remain absent. Input: since,until (RFC3339, range at most31days),after_id(optional scoped publication UUID),limit(1..100). No platform call.",
		"prepare_outbound":      "Prepare a private reply or fixed team handoff as a durable reviewed outbox item. Input: work_key (stable source-based key),kind(dm/handoff),body_artifact_id. For dm also connection_id,account_id,conversation_id. Handoff targets are restored from configuration; never provide target fields. Body JSON: text,optional subject,chinese (required for dm). Does not send.",
		"review_outbound":       "Record completed review against immutable private-reply/handoff body hash. Input: outbound_id,expected_revision,digest. Does not send.",
		"dispatch_outbound":     "Send a reviewed private reply or fixed team handoff through native authorization once, preserving provider acceptance receipt. Input: outbound_id. Unknown sends never retry; acceptance does not mean recipient read the message.",
		"read_outbound":         "Read durable private-reply/handoff state and provider receipt under the current scope. Input: outbound_id.",
		"replace_draft":         "Replace a draft before dispatch and invalidate its prior review. Input: publication_id,expected_revision,draft_artifact_id.",
		"reconcile_publication": "Reconcile an uncertain text publication using an explicit known remote ID and a fresh native read; never sends. Input: publication_id,remote_id.",
		"capture_metrics":       "Read actual platform metrics once a verified post is at least seven days old, preserving the original observation on later calls. Input: publication_id.",
		"import_image":          "Import an accessible generated image file into immutable local SOP storage, validating and preparing actual image bytes before review. Input: source_file_id (UUID). No URLs or filesystem paths.",
		"save_record":           "Persist source/fact/topic/draft/review/feedback/checkpoint/weekly_review/writing_rule content. Configuration is server-managed. Input: kind,key,content,mime,expected_revision (0 creates).",
		"read_record":           "Read a scoped durable record. Input: kind,key.",
		"list_records":          "Page scoped records. Input: kind,after_id(optional UUID),limit(1..100).",
		"read_artifact":         "Read immutable scoped UTF-8 text. Input: artifact_id. Binary media is not returned through this tool.",
		"prepare_publication":   "Prepare immutable JSON draft content for a bound account. Input: connection_id,account_id,content_key,platform(x/linkedin/facebook),draft_artifact_id,parent_remote_id(optional),parent_publication_id(optional). Does not send.",
		"review_publication":    "Record completed review against the exact draft hash. Input: publication_id,expected_revision,digest. Does not send.",
		"dispatch_publication":  "Publish reviewed content through the authorized native Connector, verify readback and archive actual English content. Input: publication_id. Archive timezone is restored from the configured publication snapshot. Unknown sends are never retried.",
		"read_publication":      "Read exact publication state and receipts. Input: publication_id.",
		"due_metrics":           "List verified posts due their one seven-day observation. Input: limit(1..100).",
		"write_daily_archive":   "Rebuild actual publication archive for one date. Input: date(YYYY-MM-DD). No internal review text is included.",
	} {
		entity := tools.ToolEntity{Identity: tools.ToolIdentity{Name: name, Author: "System", Provider: ProviderID, Label: operationLabel(name)}, Description: tools.ToolDescription{LLM: description, Human: tools.I18nText{"en_US": description}}, Parameters: []tools.ToolParameter{{Name: "input", Label: tools.I18nText{"en_US": "JSON input", "zh_Hans": "操作参数"}, LLMDescription: description + " Supply a JSON object string. Identity/owner/workspace fields are forbidden.", Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM, Required: true, SupportVariable: true}}, OutputType: "json"}
		p.RegisterTool(&journalTool{BuiltinTool: builtin.NewBuiltinTool(entity, ""), action: name, service: service, resolver: resolver})
	}
	return p
}

func operationLabel(name string) tools.I18nText {
	labels := map[string][2]string{
		"inspect_image":     {"Review image pixels", "检查配图画面"},
		"compose_image":     {"Compose branded image", "制作品牌配图"},
		"list_publications": {"Review publication history", "查看发布历史与采样"},
		"prepare_outbound":  {"Prepare reply or handoff", "准备回复或转交"}, "review_outbound": {"Review reply or handoff", "审阅回复或转交"}, "dispatch_outbound": {"Send reply or handoff", "发送回复或转交"}, "read_outbound": {"Check reply or handoff", "查看回复或转交状态"},
		"save_record": {"Save operating record", "保存运营记录"}, "read_record": {"Read operating record", "读取运营记录"}, "list_records": {"Find operating records", "查找运营记录"}, "read_artifact": {"Read saved content", "读取已保存内容"}, "import_image": {"Prepare publication image", "准备发布配图"}, "prepare_publication": {"Prepare publication", "准备发布稿"}, "review_publication": {"Record content review", "记录审稿结果"}, "replace_draft": {"Revise publication draft", "修改发布稿"}, "dispatch_publication": {"Publish and verify", "发布并核验"}, "read_publication": {"Check publication status", "查看发布状态"}, "reconcile_publication": {"Verify uncertain publication", "核实待确认的发布"}, "due_metrics": {"Find posts due for review", "查找待采样帖子"}, "capture_metrics": {"Capture seven-day metrics", "采集七天数据"}, "write_daily_archive": {"Update daily archive", "更新当日发布档案"},
	}
	label := labels[name]
	return tools.I18nText{"en_US": label[0], "zh_Hans": label[1]}
}

type journalTool struct {
	*builtin.BuiltinTool
	action   string
	service  ServiceProvider
	resolver ScopeResolver
}

func (t *journalTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &journalTool{BuiltinTool: t.BuiltinTool.ForkToolRuntime(runtime), action: t.action, service: t.service, resolver: t.resolver}
}
func invalid() error { return core.NewError(core.ErrorCodeInvalidInput, false, nil) }
func decode(raw string, out any) error {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil || decoder.Decode(new(any)) != io.EOF {
		return invalid()
	}
	return nil
}
func value(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func (t *journalTool) Invoke(ctx context.Context, user string, params map[string]interface{}, conversation, app, message *string) ([]tools.ToolInvokeMessage, error) {
	raw, ok := params["input"].(string)
	if !ok || len(params) != 1 || len(raw) > 60000 || t.service == nil || t.resolver == nil || t.Runtime() == nil {
		return nil, invalid()
	}
	scope, gateway, err := t.resolver.ResolveSocialOperations(ctx, InvocationIdentity{Runtime: t.Runtime(), ActorID: user, ConversationID: value(conversation), EmployeeID: value(app), MessageID: value(message)})
	if err != nil {
		return nil, err
	}
	service, err := t.service()
	if err != nil {
		return nil, err
	}
	if service == nil {
		return nil, invalid()
	}
	var result any
	if t.action == "inspect_image" {
		var r struct {
			ID     uuid.UUID `json:"artifact_id"`
			LogoID uuid.UUID `json:"logo_artifact_id,omitempty"`
		}
		if err = decode(raw, &r); err != nil || r.ID == uuid.Nil {
			return nil, invalid()
		}
		inspector, ok := t.resolver.(ImageInspector)
		if !ok {
			return nil, invalid()
		}
		result, err = inspector.InspectSocialImage(ctx, InvocationIdentity{Runtime: t.Runtime(), ActorID: user, ConversationID: value(conversation), EmployeeID: value(app), MessageID: value(message)}, r.ID, r.LogoID)
	} else if t.action == "import_image" {
		importer, ok := t.resolver.(ImageImporter)
		if !ok {
			return nil, invalid()
		}
		var r struct {
			ID uuid.UUID `json:"source_file_id"`
		}
		if err = decode(raw, &r); err != nil || r.ID == uuid.Nil {
			return nil, invalid()
		}
		source, readErr := importer.ImportSocialImage(ctx, InvocationIdentity{Runtime: t.Runtime(), ActorID: user, ConversationID: value(conversation), EmployeeID: value(app), MessageID: value(message)}, r.ID)
		if readErr != nil {
			return nil, readErr
		}
		if _, _, err = socialops.ImageDimensions(source); err != nil {
			return nil, err
		}
		prepared, prepareErr := socialmcp.PrepareImage(source)
		if prepareErr != nil {
			return nil, prepareErr
		}
		data, decodeErr := base64.StdEncoding.DecodeString(prepared.ImageData)
		if decodeErr != nil {
			return nil, invalid()
		}
		artifact, writeErr := service.PutArtifact(ctx, scope, prepared.MIMEType, data)
		if writeErr != nil {
			return nil, writeErr
		}
		result = map[string]any{"artifact": artifact, "width": prepared.Width, "height": prepared.Height, "bytes": prepared.Bytes}
	} else {
		result, err = t.execute(ctx, service, scope, gateway, raw)
	}
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > 60000 {
		return nil, invalid()
	}
	// Decode only the result envelope for the ordinary builtin message contract.
	// Upstream social identifiers are already strings at the native bridge.
	var data map[string]interface{}
	if json.Unmarshal(encoded, &data) != nil {
		return nil, invalid()
	}
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(data)}, nil
}
func (t *journalTool) execute(ctx context.Context, s *socialops.Service, scope socialops.Scope, gateway socialops.PublicationGateway, raw string) (any, error) {
	switch t.action {
	case "compose_image":
		var r socialops.ComposeImageRequest
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		artifact, err := s.ComposeImage(ctx, scope, r)
		return map[string]any{"artifact": artifact, "width": 1600, "height": 900, "headline": r.Headline, "body": r.Body, "layout_checked": err == nil}, err
	case "list_publications":
		var r struct {
			Since time.Time `json:"since"`
			Until time.Time `json:"until"`
			After uuid.UUID `json:"after_id,omitempty"`
			Limit int       `json:"limit"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		rows, err := s.ListPublicationObservations(ctx, scope, r.Since, r.Until, r.After, r.Limit)
		var next *uuid.UUID
		if len(rows) > 0 && len(rows) == r.Limit {
			id := rows[len(rows)-1].Publication.ID
			next = &id
		}
		return map[string]any{"items": rows, "next_after_id": next}, err
	case "prepare_outbound":
		var r struct {
			WorkKey        string    `json:"work_key"`
			Kind           string    `json:"kind"`
			ConnectionID   uuid.UUID `json:"connection_id,omitempty"`
			AccountID      string    `json:"account_id,omitempty"`
			ConversationID string    `json:"conversation_id,omitempty"`
			BodyArtifactID uuid.UUID `json:"body_artifact_id"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		out, err := s.PrepareOutbound(ctx, socialops.OutboundRequest{Scope: scope, WorkKey: r.WorkKey, Kind: r.Kind, ConnectionID: r.ConnectionID, AccountID: r.AccountID, ConversationID: r.ConversationID, BodyArtifactID: r.BodyArtifactID})
		return map[string]any{"outbound": out}, err
	case "review_outbound":
		var r struct {
			ID       uuid.UUID `json:"outbound_id"`
			Revision int64     `json:"expected_revision"`
			Digest   string    `json:"digest"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		out, err := s.ReviewOutbound(ctx, scope, r.ID, r.Revision, r.Digest)
		return map[string]any{"outbound": out}, err
	case "read_outbound", "dispatch_outbound":
		var r struct {
			ID uuid.UUID `json:"outbound_id"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		var out socialops.Outbound
		var err error
		if t.action == "read_outbound" {
			out, err = s.GetOutbound(ctx, scope, r.ID)
		} else {
			delivery, ok := gateway.(socialops.OutboundGateway)
			if !ok {
				return nil, invalid()
			}
			out, err = s.DispatchOutbound(ctx, scope, r.ID, delivery)
		}
		return map[string]any{"outbound": out}, err
	case "replace_draft":
		var r struct {
			ID         uuid.UUID `json:"publication_id"`
			Revision   int64     `json:"expected_revision"`
			ArtifactID uuid.UUID `json:"draft_artifact_id"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		p, err := s.ReplaceDraft(ctx, scope, r.ID, r.Revision, r.ArtifactID)
		return map[string]any{"publication": p}, err
	case "reconcile_publication":
		var r struct {
			ID       uuid.UUID `json:"publication_id"`
			RemoteID string    `json:"remote_id"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		p, err := s.ReconcilePublication(ctx, scope, r.ID, r.RemoteID, gateway)
		return map[string]any{"publication": p}, err
	case "capture_metrics":
		var r struct {
			ID uuid.UUID `json:"publication_id"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		m, err := s.CaptureMetrics(ctx, scope, r.ID, gateway)
		return map[string]any{"metric": m}, err
	case "save_record":
		var r struct {
			Kind             string `json:"kind"`
			Key              string `json:"key"`
			Content          string `json:"content"`
			MIME             string `json:"mime"`
			ExpectedRevision int64  `json:"expected_revision"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		if r.Kind == "configuration" || r.Kind == "archive" {
			return nil, invalid()
		}
		if r.MIME == "" {
			r.MIME = "text/markdown"
		}
		a, err := s.PutArtifact(ctx, scope, r.MIME, []byte(r.Content))
		if err != nil {
			return nil, err
		}
		record, err := s.SaveRecord(ctx, scope, r.Kind, r.Key, r.ExpectedRevision, a.ID)
		return map[string]any{"record": record, "artifact": a}, err
	case "read_record":
		var r struct {
			Kind string `json:"kind"`
			Key  string `json:"key"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		record, err := s.GetRecord(ctx, scope, r.Kind, r.Key)
		return map[string]any{"record": record}, err
	case "list_records":
		var r struct {
			Kind  string    `json:"kind"`
			After uuid.UUID `json:"after_id"`
			Limit int       `json:"limit"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		if r.Limit == 0 {
			r.Limit = 20
		}
		records, err := s.ListRecords(ctx, scope, r.Kind, r.After, r.Limit)
		return map[string]any{"items": records}, err
	case "read_artifact":
		var r struct {
			ID uuid.UUID `json:"artifact_id"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		meta, err := s.GetArtifact(ctx, scope, r.ID)
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(meta.MIME, "text/") && meta.MIME != "application/json" {
			return nil, invalid()
		}
		data, err := s.ReadArtifact(ctx, scope, r.ID)
		if err != nil {
			return nil, err
		}
		if len(data) > 50000 || !utf8.Valid(data) {
			return nil, invalid()
		}
		return map[string]any{"content": string(data), "artifact": meta}, nil
	case "prepare_publication":
		var r struct {
			AccountID           string     `json:"account_id"`
			ConnectionID        uuid.UUID  `json:"connection_id"`
			ContentKey          string     `json:"content_key"`
			Platform            string     `json:"platform"`
			ArtifactID          uuid.UUID  `json:"draft_artifact_id"`
			ParentID            string     `json:"parent_remote_id"`
			ParentPublicationID *uuid.UUID `json:"parent_publication_id"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		if r.AccountID == "" {
			return nil, invalid()
		}
		p, err := s.PreparePublication(ctx, socialops.PublicationRequest{AccountID: r.AccountID, Scope: scope, ConnectionID: r.ConnectionID, ContentKey: r.ContentKey, Platform: r.Platform, DraftArtifactID: r.ArtifactID, ParentRemoteID: r.ParentID, ParentPublicationID: r.ParentPublicationID})
		return map[string]any{"publication": p}, err
	case "review_publication":
		var r struct {
			ID       uuid.UUID `json:"publication_id"`
			Revision int64     `json:"expected_revision"`
			Digest   string    `json:"digest"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		p, err := s.ReviewPublication(ctx, scope, r.ID, r.Revision, r.Digest)
		return map[string]any{"publication": p}, err
	case "dispatch_publication":
		var r struct {
			ID uuid.UUID `json:"publication_id"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		p, err := s.Publish(ctx, scope, r.ID, gateway, "")
		return map[string]any{"publication": p}, err
	case "read_publication":
		var r struct {
			ID uuid.UUID `json:"publication_id"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		p, err := s.GetPublication(ctx, scope, r.ID)
		return map[string]any{"publication": p}, err
	case "due_metrics":
		var r struct {
			Limit int `json:"limit"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		if r.Limit == 0 {
			r.Limit = 20
		}
		posts, err := s.DueMetrics(ctx, scope, r.Limit)
		return map[string]any{"items": posts}, err
	case "write_daily_archive":
		var r struct {
			Date string `json:"date"`
		}
		if err := decode(raw, &r); err != nil {
			return nil, err
		}
		record, err := s.WriteDailyArchive(ctx, scope, r.Date)
		return map[string]any{"record": record}, err
	}
	return nil, invalid()
}
