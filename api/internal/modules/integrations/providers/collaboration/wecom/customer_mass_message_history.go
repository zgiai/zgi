package wecom

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations/core"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const maxCustomerMassMessageHistoryRangeSeconds int64 = 31 * 24 * 60 * 60

type customerMassMessageHistoryRecord struct {
	MsgID      string          `json:"msgid"`
	Creator    string          `json:"creator"`
	CreateTime json.RawMessage `json:"create_time"`
	CreateType *int            `json:"create_type"`
	Text       struct {
		Content string `json:"content"`
	} `json:"text"`
	Attachments []struct {
		MessageType string `json:"msgtype"`
	} `json:"attachments"`
}

func customerMassMessageHistoryAction() integrations.ActionDefinition {
	chatType := titled(map[string]interface{}{
		"type": "string", "enum": []interface{}{"single", "group"},
	}, "Mass-message target type", "群发目标类型")
	chatType["enum_labels_i18n"] = map[string]map[string]string{
		integrations.LocaleEnglishUS:         {"single": "Customers", "group": "Customer groups"},
		integrations.LocaleSimplifiedChinese: {"single": "客户", "group": "客户群"},
	}
	filterType := titled(map[string]interface{}{
		"type": "integer", "enum": []interface{}{0, 1, 2}, "default": 2,
	}, "Creator type (0 enterprise, 1 member, 2 all)", "创建人类型（0 企业、1 成员、2 全部）")
	filterType["enum_labels_i18n"] = map[string]map[string]string{
		integrations.LocaleEnglishUS:         {"0": "Enterprise-created", "1": "Member-created", "2": "All creators"},
		integrations.LocaleSimplifiedChinese: {"0": "企业创建", "1": "成员创建", "2": "全部创建人"},
	}
	input := object(map[string]interface{}{
		"chat_type": chatType,
		"start_time": titled(map[string]interface{}{
			"type": "integer", "minimum": 1,
		}, "Record start time (Unix seconds)", "记录开始时间（Unix 秒）"),
		"end_time": titled(map[string]interface{}{
			"type": "integer", "minimum": 1,
		}, "Record end time (Unix seconds)", "记录结束时间（Unix 秒）"),
		"creator_ref": titled(nonblank(4096), "Optional creator member reference", "可选创建人成员引用"),
		"filter_type": filterType,
		"limit": titled(map[string]interface{}{
			"type": "integer", "minimum": 1, "maximum": 100, "default": 50,
		}, "Page size", "分页数量"),
		"cursor": titled(str(2048), "Pagination cursor", "分页游标"),
	}, []string{"chat_type", "start_time", "end_time"})

	record := object(map[string]interface{}{
		"task_ref":         nonblank(4096),
		"creator_ref":      nonblank(4096),
		"created_at":       map[string]interface{}{"type": "integer", "minimum": 1},
		"source":           map[string]interface{}{"type": "string", "enum": []interface{}{"enterprise", "member", "unknown"}},
		"raw_create_type":  map[string]interface{}{"type": "integer"},
		"text_content":     str(4000),
		"attachment_types": arr(str(32), 9),
	}, []string{"task_ref", "created_at", "source", "raw_create_type", "text_content", "attachment_types"})
	output := object(map[string]interface{}{
		"provider":    str(64),
		"chat_type":   map[string]interface{}{"type": "string", "enum": []interface{}{"single", "group"}},
		"records":     arr(record, 100),
		"next_cursor": str(2048),
		"is_last":     map[string]interface{}{"type": "boolean"},
	}, []string{"provider", "chat_type", "records", "next_cursor", "is_last"})

	action := integrations.ActionDefinition{
		ID: ActionCustomerMassMessageHistoryList, ToolName: "list_wecom_customer_mass_message_history",
		Name: "List WeCom mass-message history", NameI18n: loc("List WeCom mass-message history", "列出企业微信群发历史"),
		Description: "Discover provider-visible customer or customer-group mass-message records in an explicit time range and return opaque task references for follow-up status queries.",
		DescriptionI18n: loc(
			"Discover provider-visible customer or customer-group mass-message records in an explicit time range and return opaque task references for follow-up status queries.",
			"在明确的时间范围内查找服务商可见的客户或客户群群发记录，并返回可继续查询状态的不透明任务引用。",
		),
		InputSchema: input, OutputSchema: output,
		Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow,
		DataEgress: true, ExternalDestination: "qyapi.weixin.qq.com", Idempotent: true,
		RequiredScopes: []string{ScopeCustomerMassMessage}, ScopeLabelsI18n: labels([]string{ScopeCustomerMassMessage}),
		SupportedAuthMethodIDs: []string{AuthMethodID},
		DefaultPolicy: &integrations.DefaultActionPolicy{
			Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyNeverAsk, DataEgressAllowed: true,
		},
		SupportedCallers: []tools.ToolInvokeFrom{
			tools.ToolInvokeFromAIChat,
			tools.ToolInvokeFromAgent,
			tools.ToolInvokeFromWorkflow,
		},
		IntentAliases: []string{
			"wecom mass message history", "wecom mass message records",
			"企业微信群发历史", "企业微信群发记录", "查询企微群发记录",
		},
	}
	return action
}

func (adapter *Adapter) listCustomerMassMessageHistory(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, int, string, error) {
	chatType := strings.TrimSpace(inputString(input, "chat_type"))
	if chatType != "single" && chatType != "group" {
		return nil, 0, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom mass-message history type is invalid", nil)
	}
	startTime := int64(inputInt(input, "start_time", 0))
	endTime := int64(inputInt(input, "end_time", 0))
	if startTime < 1 || endTime <= startTime || endTime-startTime > maxCustomerMassMessageHistoryRangeSeconds {
		return nil, 0, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom mass-message history time range is invalid", nil)
	}
	filterType := inputInt(input, "filter_type", 2)
	if filterType < 0 || filterType > 2 {
		return nil, 0, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom mass-message creator filter is invalid", nil)
	}
	limit := inputInt(input, "limit", 50)
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	payload := map[string]interface{}{
		"chat_type":   chatType,
		"start_time":  startTime,
		"end_time":    endTime,
		"filter_type": filterType,
		"limit":       limit,
	}
	expectedCreator := ""
	if creatorRef := strings.TrimSpace(inputString(input, "creator_ref")); creatorRef != "" {
		creator, err := adapter.decodeSubjectRef(creatorRef, creds, SubjectTypeMember, SubjectIDKindUserID)
		if err != nil {
			return nil, 0, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom mass-message creator reference is invalid", err)
		}
		expectedCreator = creator.SubjectID
		payload["creator"] = expectedCreator
	}
	if cursor := strings.TrimSpace(inputString(input, "cursor")); cursor != "" {
		payload["cursor"] = cursor
	}

	var response struct {
		apiEnvelope
		NextCursor string                             `json:"next_cursor"`
		Records    []customerMassMessageHistoryRecord `json:"group_msg_list"`
	}
	if err := adapter.authorizedJSON(ctx, creds, http.MethodPost, "/cgi-bin/externalcontact/get_groupmsg_list_v2", nil, payload, &response); err != nil {
		return nil, 0, "", err
	}
	records := make([]map[string]interface{}, 0, len(response.Records))
	for _, item := range response.Records {
		creatorID := strings.TrimSpace(item.Creator)
		if expectedCreator != "" && !strings.EqualFold(creatorID, expectedCreator) {
			return nil, 0, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom returned mass-message history for an unexpected creator", nil)
		}
		taskRef, err := adapter.encodeCustomerTaskRef(creds, item.MsgID)
		if err != nil {
			return nil, 0, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom returned an invalid mass-message task", err)
		}
		createdAt, err := parseCustomerMassMessageCreateTime(item.CreateTime)
		if err != nil {
			return nil, 0, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom returned an invalid mass-message creation time", err)
		}
		if item.CreateType == nil {
			return nil, 0, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom mass-message history did not contain a creation source", nil)
		}
		attachmentTypes := make([]string, 0, len(item.Attachments))
		if len(item.Attachments) > 9 {
			return nil, 0, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom mass-message history returned too many attachments", nil)
		}
		for _, attachment := range item.Attachments {
			messageType := bounded(attachment.MessageType, 32)
			if messageType == "" {
				return nil, 0, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom mass-message history returned an invalid attachment type", nil)
			}
			attachmentTypes = append(attachmentTypes, messageType)
		}
		projected := map[string]interface{}{
			"task_ref":         taskRef,
			"created_at":       createdAt,
			"source":           customerMassMessageCreateSource(*item.CreateType),
			"raw_create_type":  *item.CreateType,
			"text_content":     bounded(item.Text.Content, 4000),
			"attachment_types": attachmentTypes,
		}
		if creatorID != "" {
			creatorRef, encodeErr := adapter.encodeSubjectRef(creds, SubjectTypeMember, SubjectIDKindUserID, creatorID, "", "")
			if encodeErr != nil {
				return nil, 0, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom returned an invalid mass-message creator", encodeErr)
			}
			projected["creator_ref"] = creatorRef
		}
		records = append(records, projected)
	}
	nextCursor := bounded(response.NextCursor, 2048)
	output := map[string]interface{}{
		"provider":    IntegrationID,
		"chat_type":   chatType,
		"records":     records,
		"next_cursor": nextCursor,
		"is_last":     strings.TrimSpace(nextCursor) == "",
	}
	return output, len(records), firstNonEmpty(response.RequestID, response.RequestIDAlt), nil
}

func parseCustomerMassMessageCreateTime(raw json.RawMessage) (int64, error) {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return 0, fmt.Errorf("missing creation time")
	}
	if strings.HasPrefix(value, "\"") {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return 0, err
		}
		value = strings.TrimSpace(text)
	}
	timestamp, err := strconv.ParseInt(value, 10, 64)
	if err != nil || timestamp < 1 {
		return 0, fmt.Errorf("invalid creation time")
	}
	return timestamp, nil
}

func customerMassMessageCreateSource(raw int) string {
	switch raw {
	case 0:
		return "enterprise"
	case 1:
		return "member"
	default:
		return "unknown"
	}
}

func customerMassMessageTaskStatus(raw int) string {
	switch raw {
	case 0:
		return "pending_member_send"
	case 2:
		return "sent"
	default:
		return "unknown"
	}
}

func customerMassMessageResultStatus(raw int) string {
	switch raw {
	case 0:
		return "pending"
	case 1:
		return "sent"
	case 2:
		return "failed_customer_not_friend"
	case 3:
		return "failed_duplicate_mass_message"
	default:
		return "unknown"
	}
}

func projectCustomerMassMessageSentAt(output map[string]interface{}, status string, sentAt *int64) error {
	if sentAt == nil || status != "sent" {
		return nil
	}
	if *sentAt < 1 {
		return fmt.Errorf("invalid send time")
	}
	output["sent_at"] = *sentAt
	return nil
}
