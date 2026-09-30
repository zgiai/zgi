package feishu

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

// Local protection, not a provider page-size guarantee. Feishu groups users
// joining simultaneously, so one upstream page can exceed page_size (or 100).
const maxChatMembersPerPage = 1000

func readActionResult(operation string, data, args map[string]any, secrets []string) (map[string]any, error) {
	if operation == "feishu.tenant.get" {
		return tenantResult(data, secrets)
	}
	return chatMembersResult(data, args, secrets)
}

func tenantResult(data map[string]any, secrets []string) (map[string]any, error) {
	tenant, ok := data["tenant"].(map[string]any)
	if !ok {
		return nil, failure(core.ErrorCodeProviderUnavailable)
	}
	name, nameOK := readDisplayName(tenant["name"], secrets)
	displayID, displayOK := readIdentity(tenant["display_id"], 128, secrets)
	tenantKey, keyOK := readIdentity(tenant["tenant_key"], 128, secrets)
	if !nameOK || !displayOK || !keyOK {
		return nil, failure(core.ErrorCodeProviderUnavailable)
	}
	result := map[string]any{"name": name, "display_id": displayID, "tenant_key": tenantKey}
	if raw, exists := tenant["tenant_tag"]; exists {
		tag, ok := raw.(float64)
		if !ok || math.Trunc(tag) != tag || math.IsInf(tag, 0) || math.IsNaN(tag) {
			return nil, failure(core.ErrorCodeProviderUnavailable)
		}
		// Keep unknown integer tags without inventing a team/personal label.
		result["tenant_tag"] = tag
	}
	return map[string]any{"tenant": result}, nil
}

func chatMembersResult(data, args map[string]any, secrets []string) (map[string]any, error) {
	items, itemsOK := data["items"].([]any)
	hasMore, moreOK := data["has_more"].(bool)
	if !itemsOK || !moreOK || len(items) > maxChatMembersPerPage {
		return nil, failure(core.ErrorCodeProviderUnavailable)
	}
	cursor := ""
	if raw, exists := data["page_token"]; exists {
		var ok bool
		cursor, ok = raw.(string)
		if !ok || utf8.RuneCountInString(cursor) > 1024 || containsSecret(cursor, secrets) || containsControl(cursor) {
			return nil, failure(core.ErrorCodeProviderUnavailable)
		}
	}
	if hasMore && (strings.TrimSpace(cursor) == "" || cursor == providers.Text(args, "page_token")) {
		return nil, failure(core.ErrorCodeProviderUnavailable)
	}
	members := make([]any, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok || item["member_id_type"] != "open_id" {
			return nil, failure(core.ErrorCodeProviderUnavailable)
		}
		id, idOK := readIdentity(item["member_id"], 128, secrets)
		name, nameOK := readDisplayName(item["name"], secrets)
		if !idOK || !validMemberOpenID(id) || !nameOK {
			return nil, failure(core.ErrorCodeProviderUnavailable)
		}
		members = append(members, map[string]any{"open_id": id, "name": name})
	}
	result := map[string]any{"members": members, "has_more": hasMore}
	if hasMore {
		result["page_token"] = cursor
	}
	return result, nil
}

func readDisplayName(raw any, secrets []string) (string, bool) {
	name, ok := raw.(string)
	if !ok || strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 256 {
		return "", false
	}
	name = providers.SafeText(name, 256)
	for _, secret := range secrets {
		if secret != "" {
			name = strings.ReplaceAll(name, secret, "[redacted]")
		}
	}
	// Redaction can grow short secret strings; never silently truncate a field.
	return name, strings.TrimSpace(name) != "" && utf8.RuneCountInString(name) <= 256
}

func readIdentity(raw any, limit int, secrets []string) (string, bool) {
	value, ok := raw.(string)
	return value, ok && strings.TrimSpace(value) != "" && utf8.RuneCountInString(value) <= limit && !containsControl(value) && !containsSecret(value, secrets)
}

func containsSecret(value string, secrets []string) bool {
	for _, secret := range secrets {
		if secret != "" && strings.Contains(value, secret) {
			return true
		}
	}
	return false
}

func containsControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}

func validMemberOpenID(id string) bool {
	if !strings.HasPrefix(id, "ou_") || len(id) <= 3 {
		return false
	}
	for _, r := range id[3:] {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// These business codes belong only to these two endpoints. In particular,
// never classify an unrecognized result from an existing write as safe to replay.
func operationAPIFailure(operation string, code int, requestID string, write bool) error {
	var classified core.ErrorCode
	switch operation {
	case "feishu.tenant.get":
		if code == 1184000 || code == 1184001 {
			classified = core.ErrorCodeResourceAccessDenied
		}
	case "feishu.chat.members.list":
		switch code {
		case 232001, 232006:
			classified = core.ErrorCodeInvalidInput
		case 232009, 232010, 232011, 232033:
			classified = core.ErrorCodeResourceAccessDenied
		case 232004, 232025, 232034:
			classified = core.ErrorCodeApplicationNotReady
		}
	}
	if classified != "" {
		return &providers.Failure{Code: classified, RequestID: requestID}
	}
	return apiFailure(code, requestID, write)
}
