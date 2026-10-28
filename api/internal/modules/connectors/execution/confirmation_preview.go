package execution

import (
	"context"
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/zgiai/zgi/api/internal/capabilities/serverops"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// ConfirmationPreview is private presentation of a frozen request. It is only
// returned by the owner-authorized receipt query, never by runtime events.
type ConfirmationPreview struct {
	AccountName     string                     `json:"account_name"`
	AccountIdentity string                     `json:"account_identity"`
	Kind            string                     `json:"kind"`
	Fields          []ConfirmationPreviewField `json:"fields"`
	Complete        bool                       `json:"complete"`
}

type ConfirmationPreviewField struct {
	Label     string                `json:"label"`
	LabelI18n catalog.LocalizedText `json:"label_i18n"`
	Value     string                `json:"value"`
	Truncated bool                  `json:"truncated"`
	// Format is an optional presentation hint. ConfirmationPreviewFormatCommand
	// marks a command line the client renders as code.
	Format string `json:"format,omitempty"`
}

// ConfirmationPreviewFormatCommand marks a preview field whose value is the
// exact command line an action runs.
const ConfirmationPreviewFormatCommand = "command"

// ConfirmationTargetReader resolves local, scoped conversation handles without
// logging in, invoking the provider, or exposing a context token.
type ConfirmationTargetReader interface {
	ReadConfirmationTarget(context.Context, ProviderInvocation) (string, error)
}

type previewFieldRule struct {
	key, en, zh string
	limit       int
	required    bool
}

func previewRules(id string) (string, []previewFieldRule, bool) {
	text := previewFieldRule{"text", "Content", "内容", 2048, true}
	field := func(key, en, zh string) previewFieldRule { return previewFieldRule{key, en, zh, 4096, true} }
	switch id {
	case "gmail.message.send", "qqmail.message.send", "netease163.message.send":
		return "mail", []previewFieldRule{{"to", "Recipients", "收件人", 8192, true}, {"subject", "Subject", "主题", 998, true}, text}, true
	case "dingtalk.messages.send_direct":
		return "message", []previewFieldRule{field("user_ids", "Recipients", "收件人"), text}, true
	case "dingtalk.messages.send_group":
		return "message", []previewFieldRule{field("conversation_id", "Conversation", "会话"), text}, true
	case "wecom.messages.send", "weixin.messages.reply":
		return "message", []previewFieldRule{field("resolved_recipient", "Conversation recipient", "会话接收方"), text}, true
	case "feishu.message.batch_send":
		return "message", []previewFieldRule{field("open_ids", "Recipients", "收件人"), text}, true
	case "feishu.message.system_send":
		return "message", []previewFieldRule{field("open_id", "Recipient", "收件人"), text}, true
	case "feishu.message.send":
		return "message", []previewFieldRule{field("receive_id_type", "Recipient ID type", "接收方标识类型"), field("receive_id", "Recipient", "接收方"), text}, true
	case "feishu.message.reply", "feishu.message.update":
		return "message", []previewFieldRule{field("message_id", "Message", "消息"), text}, true
	case "feishu.document.blocks.append", "feishu.document.blocks.update":
		return "resource", []previewFieldRule{field("document_id", "Document", "文档"), field("block_id", "Block", "内容块"), text}, true
	case "feishu.comment.create", "feishu.comment.reply", "feishu.comment.update", "feishu.comment.delete":
		rules := []previewFieldRule{field("file_token", "File", "文件")}
		if id != "feishu.comment.create" {
			rules = append(rules, field("comment_id", "Comment", "评论"))
		}
		if id == "feishu.comment.update" || id == "feishu.comment.delete" {
			rules = append(rules, field("reply_id", "Reply", "回复"))
		}
		if id != "feishu.comment.delete" {
			rules = append(rules, text)
		}
		return "resource", rules, true
	case "feishu.pin.add", "feishu.pin.remove":
		return "resource", []previewFieldRule{field("message_id", "Message", "消息")}, true
	case "feishu.reaction.add":
		return "resource", []previewFieldRule{field("message_id", "Message", "消息"), field("emoji_type", "Reaction", "表情")}, true
	case "feishu.reaction.remove":
		return "resource", []previewFieldRule{field("message_id", "Message", "消息"), field("reaction_id", "Reaction", "表情")}, true
	case "feishu.chat.create":
		return "resource", []previewFieldRule{field("name", "Name", "名称"), field("user_id_list", "Members", "成员")}, true
	case "feishu.chat.update":
		return "resource", []previewFieldRule{field("chat_id", "Chat", "群聊"), field("name", "Name", "名称"), {"description", "Description", "说明", 1000, false}}, true
	case "feishu.card.create", "feishu.document.blocks.convert":
		return "resource", []previewFieldRule{text}, true
	case "feishu.card.update":
		return "resource", []previewFieldRule{field("card_id", "Card", "卡片"), field("sequence", "Sequence", "版本序号"), text}, true
	case "feishu.resource.upload":
		return "resource", []previewFieldRule{field("file_id", "File ID", "文件标识")}, true
	case "github.issue.create", "github.issue.comment.create":
		rules := []previewFieldRule{field("owner", "Repository owner", "仓库所有者"), field("repo", "Repository", "仓库")}
		if id == "github.issue.create" {
			rules = append(rules, field("title", "Title", "标题"), previewFieldRule{"body", "Content", "内容", 2048, false}, previewFieldRule{"assignees", "Assignees", "经办人", 4096, false}, previewFieldRule{"labels", "Labels", "标签", 4096, false})
		} else {
			rules = append(rules, field("issue_number", "Issue number", "议题编号"), previewFieldRule{"body", "Content", "内容", 2048, true})
		}
		return "resource", rules, true
	default:
		return "resource", nil, false
	}
}

func previewText(value string, limit int) (string, bool) {
	// Keep plain text readable; remove controls and bidirectional overrides.
	original := value
	value = strings.Map(func(r rune) rune {
		if (unicode.IsControl(r) && r != '\n' && r != '\t') || unicode.In(r, unicode.Cf) {
			return -1
		}
		return r
	}, value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit]), true
	}
	return value, value != original
}

func buildConfirmationPreview(request ExecutionRequest, connection core.PersonalConnection, recipient string) *ConfirmationPreview {
	if catalog.IsSocialMCPConnector(request.ConnectorID.String()) {
		return buildSocialConfirmationPreview(request, connection)
	}
	if request.ConnectorID.String() == catalog.ServerOpsSSHConnectorID {
		return buildServerOpsSSHConfirmationPreview(request, connection)
	}
	kind, rules, known := previewRules(string(request.OperationID))
	p := &ConfirmationPreview{Kind: kind, Fields: []ConfirmationPreviewField{}, Complete: known}
	p.AccountName, _ = previewText(connection.Name, 128)
	if connection.ExternalAccountID != nil {
		var altered bool
		p.AccountIdentity, altered = previewText(*connection.ExternalAccountID, 512)
		p.Complete = p.Complete && !altered
	}
	if connection.ProviderEvidence != nil && connection.ProviderEvidence.Mail != nil {
		var altered bool
		p.AccountIdentity, altered = previewText(connection.ProviderEvidence.Mail.AuthenticatedLogin, 254)
		p.Complete = known && !altered
	}
	if p.AccountIdentity == "" {
		p.Complete = false
	}
	encoded, err := json.Marshal(request.Input)
	if err != nil {
		p.Complete = false
		return p
	}
	defer clear(encoded)
	var input map[string]any
	if json.Unmarshal(encoded, &input) != nil {
		p.Complete = false
		return p
	}
	// This key is local-only and can never be supplied by operation arguments.
	delete(input, "resolved_recipient")
	if recipient != "" {
		input["resolved_recipient"] = recipient
	}
	for _, rule := range rules {
		v, exists := input[rule.key]
		if !exists {
			if rule.required {
				p.Complete = false
			}
			continue
		}
		var value string
		switch v := v.(type) {
		case string:
			value = v
		case float64:
			value = strconv.FormatFloat(v, 'f', -1, 64)
		case []any:
			if len(v) > 20 {
				p.Complete = false
				continue
			}
			parts := make([]string, 0, len(v))
			for _, entry := range v {
				s, ok := entry.(string)
				if !ok {
					p.Complete = false
					continue
				}
				parts = append(parts, s)
			}
			value = strings.Join(parts, "\n")
		default:
			p.Complete = false
			continue
		}
		text, truncated := previewText(value, rule.limit)
		if truncated && rule.key != "text" && rule.key != "body" && rule.key != "description" {
			p.Complete = false
		}
		if text == "" && rule.required && rule.key != "subject" {
			p.Complete = false
		}
		p.Fields = append(p.Fields, ConfirmationPreviewField{Label: rule.en, LabelI18n: catalog.LocalizedText{"en-US": rule.en, "zh-Hans": rule.zh}, Value: text, Truncated: truncated})
	}
	return p
}

// serverOpsSSHServiceNamePattern is the capability's exact systemd unit
// grammar. It is compiled once from the exported constant so the preview can
// never advertise a looser service_name than the driver accepts.
var serverOpsSSHServiceNamePattern = regexp.MustCompile(serverops.SystemdServiceNamePattern)

// buildServerOpsSSHConfirmationPreview builds the closed, complete preview for
// the SSH Server Operations connector. The preview is derived only from the
// frozen operation arguments, the already-verified non-secret connection
// identity, and the connection's non-secret privilege evidence; credentials,
// private keys, passphrases, and remote output are never read or emitted.
//
// Every action, read or write, carries a Command field: the exact command line
// it runs, rendered by serverops.CommandLine from the same pieces the
// capability executes, so the approver sees what will run. A write action also
// shows its Action. The snapshot action declares no arguments, so it shows the
// fixed action and pinned target. Every other action must supply exactly its
// declared arguments: an unknown key, a wrong type, an out-of-range integer, a
// malformed service_name, or privilege evidence that does not belong to this
// connection marks the preview incomplete rather than being normalized.
func buildServerOpsSSHConfirmationPreview(request ExecutionRequest, connection core.PersonalConnection) *ConfirmationPreview {
	preview := &ConfirmationPreview{Kind: "resource", Fields: []ConfirmationPreviewField{}, Complete: true}
	preview.AccountName, _ = previewText(connection.Name, 128)
	identity, identityComplete := serverOpsSSHAccountIdentity(connection.ExternalAccountID)
	preview.AccountIdentity = identity
	if !identityComplete {
		preview.Complete = false
	}

	tool, known := serverOpsSSHTool(request.OperationID.String())
	if !known {
		preview.Complete = false
		return preview
	}
	input, ok := serverOpsSSHInputObject(request.Input)
	if !ok {
		preview.Complete = false
		return preview
	}
	parameters := make(map[string]serverops.ParameterDescriptor, len(tool.Parameters))
	for _, parameter := range tool.Parameters {
		parameters[parameter.Name] = parameter
	}
	privilege, privilegeOK := serverOpsSSHPrivilege(connection, identity)
	if !privilegeOK {
		preview.Complete = false
		return preview
	}

	switch tool.ActionID {
	case serverops.ActionSystemSnapshot:
		if len(input) != 0 {
			preview.Complete = false
			return preview
		}
		preview.Fields = append(preview.Fields,
			serverOpsSSHPreviewField("Action", "动作", serverops.ActionSystemSnapshot),
			serverOpsSSHPreviewField("Target", "目标", identity),
		)
	case serverops.ActionServiceStatus:
		if !serverOpsSSHOnlyKeys(input, "service_name") {
			preview.Complete = false
			return preview
		}
		name, nameOK := serverOpsSSHServiceName(input, parameters)
		if !nameOK {
			preview.Complete = false
			return preview
		}
		preview.Fields = append(preview.Fields, serverOpsSSHPreviewField("Service", "服务", name))
	case serverops.ActionJournalTail:
		if !serverOpsSSHOnlyKeys(input, "service_name", "lines", "since_minutes") {
			preview.Complete = false
			return preview
		}
		name, nameOK := serverOpsSSHServiceName(input, parameters)
		lines, linesOK := serverOpsSSHInteger(input, parameters, "lines")
		sinceMinutes, sinceOK := serverOpsSSHInteger(input, parameters, "since_minutes")
		if !nameOK || !linesOK || !sinceOK {
			preview.Complete = false
			return preview
		}
		preview.Fields = append(preview.Fields,
			serverOpsSSHPreviewField("Service", "服务", name),
			serverOpsSSHPreviewField("Lines", "行数", strconv.FormatInt(lines, 10)),
			serverOpsSSHPreviewField("Since minutes", "时间范围（分钟）", strconv.FormatInt(sinceMinutes, 10)),
		)
	case serverops.ActionServiceRestart, serverops.ActionServiceStart, serverops.ActionServiceStop, serverops.ActionServiceReload:
		if !serverOpsSSHOnlyKeys(input, "service_name") {
			preview.Complete = false
			return preview
		}
		name, nameOK := serverOpsSSHServiceName(input, parameters)
		if !nameOK {
			preview.Complete = false
			return preview
		}
		preview.Fields = append(preview.Fields,
			serverOpsSSHPreviewField("Action", "动作", tool.ActionID),
			serverOpsSSHPreviewField("Service", "服务", name),
		)
	default:
		preview.Complete = false
		return preview
	}

	// The command line is rendered from the same validated input and the same
	// builders the capability executes. Any disagreement fails closed.
	command, err := serverops.CommandLine(tool.ActionID, input, privilege)
	if err != nil {
		preview.Complete = false
		return preview
	}
	commandField := serverOpsSSHPreviewField("Command", "执行命令", command)
	commandField.Format = ConfirmationPreviewFormatCommand
	preview.Fields = append(preview.Fields, commandField)
	return preview
}

// serverOpsSSHPrivilege returns the connection's privilege from its verified,
// non-secret evidence. A connection verified before the privilege existed has
// no SSH evidence and runs as the SSH user. Evidence that names another
// identity or an unsupported privilege is rejected.
func serverOpsSSHPrivilege(connection core.PersonalConnection, identity string) (serverops.Privilege, bool) {
	if connection.ProviderEvidence == nil || connection.ProviderEvidence.ServerOps == nil {
		return serverops.PrivilegeNone, true
	}
	evidence := connection.ProviderEvidence.ServerOps
	privilege := serverops.Privilege(evidence.Privilege)
	if evidence.Version != 1 || evidence.Identity == "" || evidence.Identity != identity || !privilege.Valid() {
		return serverops.PrivilegeNone, false
	}
	return privilege, true
}

// serverOpsSSHTool resolves the declared tool contract for one action. The
// capability manifest is the single source of truth for the accepted argument
// names, pattern, bounds, and defaults.
func serverOpsSSHTool(actionID string) (serverops.ToolDescriptor, bool) {
	for _, tool := range serverops.Skill().Tools {
		if tool.ActionID == actionID {
			return tool, true
		}
	}
	return serverops.ToolDescriptor{}, false
}

// serverOpsSSHAccountIdentity returns the verified, non-secret "username@host:port"
// identity the adapter stored. A missing, empty, truncated, or altered value is
// reported incomplete so a governed DigitalHuman approval can never rely on an
// absent or rewritten target identity.
func serverOpsSSHAccountIdentity(external *string) (string, bool) {
	if external == nil {
		return "", false
	}
	value, alteredOrTruncated := previewText(*external, 512)
	if value == "" || alteredOrTruncated {
		return value, false
	}
	return value, true
}

// serverOpsSSHInputObject normalizes the frozen arguments to a JSON object
// without retaining the raw encoding. A non-object value is rejected.
func serverOpsSSHInputObject(input any) (map[string]any, bool) {
	if input == nil {
		return map[string]any{}, true
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil, false
	}
	defer clear(encoded)
	var object map[string]any
	if json.Unmarshal(encoded, &object) != nil {
		return nil, false
	}
	if object == nil {
		object = map[string]any{}
	}
	return object, true
}

// serverOpsSSHOnlyKeys reports whether every supplied argument is declared.
func serverOpsSSHOnlyKeys(input map[string]any, allowed ...string) bool {
	for key := range input {
		matched := false
		for _, candidate := range allowed {
			if key == candidate {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// serverOpsSSHServiceName validates service_name against the exact declared
// grammar and returns it unchanged. The value is never trimmed or rewritten.
func serverOpsSSHServiceName(input map[string]any, parameters map[string]serverops.ParameterDescriptor) (string, bool) {
	descriptor, ok := parameters["service_name"]
	if !ok || descriptor.Kind != serverops.ParameterKindString || !descriptor.Required {
		return "", false
	}
	raw, exists := input["service_name"]
	if !exists {
		return "", false
	}
	value, ok := raw.(string)
	if !ok || !serverOpsSSHServiceNamePattern.MatchString(value) {
		return "", false
	}
	return value, true
}

// serverOpsSSHInteger resolves one optional integer argument, applying the
// declared default when it is omitted and rejecting a wrong type or an
// out-of-range value instead of clamping it.
func serverOpsSSHInteger(input map[string]any, parameters map[string]serverops.ParameterDescriptor, name string) (int64, bool) {
	descriptor, ok := parameters[name]
	if !ok || descriptor.Kind != serverops.ParameterKindInteger {
		return 0, false
	}
	raw, exists := input[name]
	if !exists {
		if descriptor.HasDefault {
			return descriptor.Default, true
		}
		return 0, false
	}
	value, ok := serverOpsSSHIntegerValue(raw)
	if !ok || value < descriptor.Minimum || value > descriptor.Maximum {
		return 0, false
	}
	return value, true
}

func serverOpsSSHIntegerValue(raw any) (int64, bool) {
	switch value := raw.(type) {
	case int:
		return int64(value), true
	case int32:
		return int64(value), true
	case int64:
		return value, true
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) || value != math.Trunc(value) ||
			value < math.MinInt64 || value > math.MaxInt64 {
			return 0, false
		}
		return int64(value), true
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

// serverOpsSSHPreviewField builds one parser-safe localized field.
func serverOpsSSHPreviewField(en, zh, value string) ConfirmationPreviewField {
	return ConfirmationPreviewField{
		Label:     en,
		LabelI18n: catalog.LocalizedText{"en-US": en, "zh-Hans": zh},
		Value:     value,
	}
}

// confirmationPreview builds the frozen presentation from the pinned request
// and the current connection. It is shared by the presentation and
// confirmed-continuation paths so both rebuild the exact same preview.
func (service *Service) confirmationPreview(ctx context.Context, pinned PinnedExecution, binding core.TargetBinding, connection core.PersonalConnection) *ConfirmationPreview {
	var recipient string
	if reader, ok := service.provider.(ConfirmationTargetReader); ok && (pinned.Request.OperationID == "wecom.messages.send" || pinned.Request.OperationID == "weixin.messages.reply") {
		recipient, _ = reader.ReadConfirmationTarget(ctx, ProviderInvocation{OrganizationID: connection.OrganizationID, OwnerAccountID: connection.OwnerAccountID, ConnectionID: connection.ID, ConnectorID: connection.ConnectorID, CredentialVersion: connection.CredentialVersion, BindingID: binding.ID, BindingRevision: binding.Revision, Target: pinned.Request.Target, OperationID: pinned.Request.OperationID, Input: pinned.Request.Input})
	}
	return buildConfirmationPreview(pinned.Request, connection, recipient)
}

func (service *Service) confirmationWithPreview(ctx context.Context, pinned PinnedExecution, accepted catalog.PinnedOperation, binding core.TargetBinding, connection core.PersonalConnection, risk RiskClassification) *RuntimeConfirmation {
	confirmation := runtimeConfirmation(pinned.Receipt, accepted, pinned.Request.Target, risk)
	confirmation.Preview = service.confirmationPreview(ctx, pinned, binding, connection)
	return confirmation
}

// completeDigitalHumanPreview mirrors the web parser's completeness rule. A
// DigitalHuman approval is never presented or accepted without a complete,
// non-secret preview: a nil preview, an explicit incomplete marker, a missing
// account identity, or an empty field set is treated as incomplete.
func completeDigitalHumanPreview(preview *ConfirmationPreview) bool {
	return preview != nil && preview.Complete && preview.AccountIdentity != "" && len(preview.Fields) > 0
}

// digitalHumanApprovalPreviewAllowed is the single server-side rule for
// presenting or accepting an approval. The shared Chat/Agent/Workflow path is
// returned unchanged; only a governed DigitalHuman invocation must carry a
// complete preview.
func digitalHumanApprovalPreviewAllowed(decision digitalHumanApprovalDecision, preview *ConfirmationPreview) bool {
	if !decision.governs() {
		return true
	}
	return completeDigitalHumanPreview(preview)
}
