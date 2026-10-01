package execution

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"unicode"

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
}

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

func (service *Service) confirmationWithPreview(ctx context.Context, pinned PinnedExecution, accepted catalog.PinnedOperation, binding core.TargetBinding, connection core.PersonalConnection, risk RiskClassification) *RuntimeConfirmation {
	confirmation := runtimeConfirmation(pinned.Receipt, accepted, pinned.Request.Target, risk)
	var recipient string
	if reader, ok := service.provider.(ConfirmationTargetReader); ok && (pinned.Request.OperationID == "wecom.messages.send" || pinned.Request.OperationID == "weixin.messages.reply") {
		recipient, _ = reader.ReadConfirmationTarget(ctx, ProviderInvocation{OrganizationID: connection.OrganizationID, OwnerAccountID: connection.OwnerAccountID, ConnectionID: connection.ID, ConnectorID: connection.ConnectorID, CredentialVersion: connection.CredentialVersion, BindingID: binding.ID, BindingRevision: binding.Revision, Target: pinned.Request.Target, OperationID: pinned.Request.OperationID, Input: pinned.Request.Input})
	}
	confirmation.Preview = buildConfirmationPreview(pinned.Request, connection, recipient)
	return confirmation
}
