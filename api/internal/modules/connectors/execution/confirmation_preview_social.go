package execution

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// Social previews use only the deployment-owned operation schema and frozen
// arguments. Unknown fields never enter the presentation or make it complete.
func buildSocialConfirmationPreview(request ExecutionRequest, connection core.PersonalConnection) *ConfirmationPreview {
	p := &ConfirmationPreview{Kind: "resource", Fields: []ConfirmationPreviewField{}}
	p.AccountName, _ = previewText(connection.Name, 128)
	if connection.ExternalAccountID == nil {
		return p
	}
	identity, altered := previewText(*connection.ExternalAccountID, 512)
	p.AccountIdentity = identity
	if identity == "" || altered {
		return p
	}
	var operation *catalog.Operation
	for _, definition := range catalog.SocialMCPDefinitions() {
		if definition.ID != request.ConnectorID.String() {
			continue
		}
		for _, candidate := range definition.Operations {
			if candidate.ID == request.OperationID.String() {
				copy := candidate
				operation = &copy
				break
			}
		}
	}
	if operation == nil {
		return p
	}
	encoded, err := json.Marshal(request.Input)
	if err != nil {
		return p
	}
	defer clear(encoded)
	var input map[string]any
	if json.Unmarshal(encoded, &input) != nil || input == nil {
		return p
	}
	properties, ok := operation.InputSchema["properties"].(map[string]any)
	if !ok {
		return p
	}
	p.Complete = true
	for key := range input {
		if _, exists := properties[key]; !exists {
			p.Complete = false
		}
	}
	for _, raw := range operation.InputSchema["required"].([]any) {
		key, ok := raw.(string)
		if !ok {
			p.Complete = false
			continue
		}
		if _, exists := input[key]; !exists {
			p.Complete = false
		}
	}
	p.Fields = append(p.Fields, ConfirmationPreviewField{Label: "Action", LabelI18n: catalog.LocalizedText{"en-US": "Action", "zh-Hans": "动作"}, Value: operation.Name})
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	labels := map[string][2]string{
		"account_id": {"Publishing account", "运营账号"}, "post_id": {"Post", "帖子"}, "comment_id": {"Comment", "评论"}, "conversation_id": {"Private conversation", "私信会话"}, "text": {"Content", "内容"}, "image_url": {"Image URL", "图片地址"}, "mime_type": {"Image type", "图片类型"}, "query": {"Search query", "搜索词"}, "limit": {"Limit", "条数"}, "cursor": {"Page cursor", "分页游标"}, "since": {"Since", "开始时间"}, "until": {"Until", "结束时间"}, "since_id": {"Since ID", "起始标识"}, "until_id": {"Until ID", "结束标识"}, "metrics": {"Metrics", "指标"},
	}
	for _, key := range keys {
		value, exists := input[key]
		if !exists {
			continue
		}
		schema, ok := properties[key].(map[string]any)
		if !ok {
			p.Complete = false
			continue
		}
		var display string
		switch schema["type"] {
		case "string":
			s, ok := value.(string)
			if !ok || s == "" {
				p.Complete = false
				continue
			}
			display = s
		case "integer":
			n, ok := value.(float64)
			if !ok || n != float64(int64(n)) {
				p.Complete = false
				continue
			}
			b, _ := json.Marshal(n)
			display = string(b)
		case "boolean":
			b, ok := value.(bool)
			if !ok {
				p.Complete = false
				continue
			}
			raw, _ := json.Marshal(b)
			display = string(raw)
		case "array":
			values, ok := value.([]any)
			if !ok {
				p.Complete = false
				continue
			}
			valid := true
			for _, v := range values {
				if _, ok := v.(string); !ok {
					valid = false
				}
			}
			if !valid {
				p.Complete = false
				continue
			}
			raw, _ := json.Marshal(values)
			display = string(raw)
		default:
			p.Complete = false
			continue
		}
		label := labels[key]
		if label[0] == "" {
			label = [2]string{key, key}
		}
		// Never put the base64 attachment into the approval card. Its digest pins
		// the exact frozen bytes while normal schema/media validation remains required.
		if key == "image_data" {
			image, err := base64.StdEncoding.DecodeString(display)
			if err != nil || len(image) == 0 {
				p.Complete = false
				continue
			}
			digest := sha256.Sum256(image)
			clear(image)
			display = hex.EncodeToString(digest[:])
			label = [2]string{"Image SHA-256", "图片 SHA-256"}
		}
		safe, changed := previewText(display, 8192)
		if changed {
			p.Complete = false
		}
		p.Fields = append(p.Fields, ConfirmationPreviewField{Label: label[0], LabelI18n: catalog.LocalizedText{"en-US": label[0], "zh-Hans": label[1]}, Value: safe, Truncated: changed})
	}
	return p
}
