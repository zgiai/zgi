package feishu

import (
	"strings"
	"unicode/utf8"

	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

// Project only the two requested fields. A directory response can contain
// private HR fields or per-field permission errors even when code is zero.
// Neither may become a successful empty search or an implicit recipient choice.
func searchUsersResult(data, args map[string]any) (map[string]any, error) {
	if raw := data["abnormals"]; raw != nil {
		abnormals, ok := raw.([]any)
		if !ok {
			return nil, failure(core.ErrorCodeProviderUnavailable)
		}
		for _, item := range abnormals {
			record, ok := item.(map[string]any)
			if !ok {
				return nil, failure(core.ErrorCodeProviderUnavailable)
			}
			if raw, exists := record["row_error"]; exists {
				code, ok := raw.(float64)
				if code == 1000 {
					return nil, failure(core.ErrorCodePermissionMissing)
				}
				if !ok || code != 0 {
					return nil, failure(core.ErrorCodeProviderUnavailable)
				}
			}
			fields, ok := record["field_errors"].(map[string]any)
			if !ok && record["field_errors"] != nil {
				return nil, failure(core.ErrorCodeProviderUnavailable)
			}
			for _, raw := range fields {
				code, ok := raw.(float64)
				if code == 1000 {
					return nil, failure(core.ErrorCodePermissionMissing)
				}
				if !ok || code != 0 {
					return nil, failure(core.ErrorCodeProviderUnavailable)
				}
			}
		}
	}
	page, ok := data["page_response"].(map[string]any)
	if !ok {
		return nil, failure(core.ErrorCodeProviderUnavailable)
	}
	hasMore, ok := page["has_more"].(bool)
	if !ok {
		return nil, failure(core.ErrorCodeProviderUnavailable)
	}
	token := providers.Text(page, "page_token")
	if len(token) > 1024 || (hasMore && (token == "" || token == providers.Text(args, "page_token"))) {
		return nil, failure(core.ErrorCodeProviderUnavailable)
	}
	employees, ok := data["employees"].([]any)
	if !ok && data["employees"] != nil {
		return nil, failure(core.ErrorCodeProviderUnavailable)
	}
	limit := providers.Number(args, "page_size")
	if limit == 0 {
		limit = 20
	}
	if len(employees) > limit || (hasMore && len(employees) == 0) {
		return nil, failure(core.ErrorCodeProviderUnavailable)
	}
	users := make([]any, 0, len(employees))
	seen := map[string]bool{}
	for _, raw := range employees {
		employee, _ := raw.(map[string]any)
		base, _ := employee["base_info"].(map[string]any)
		names, _ := base["name"].(map[string]any)
		localized, _ := names["name"].(map[string]any)
		name := providers.Text(localized, "default_value")
		id := providers.Text(base, "employee_id")
		if !strings.HasPrefix(id, "ou_") || len(id) > 128 || strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 256 {
			return nil, failure(core.ErrorCodeProviderUnavailable)
		}
		if !seen[id] {
			users = append(users, map[string]any{"open_id": id, "name": name})
			seen[id] = true
		}
	}
	result := map[string]any{"users": users, "has_more": hasMore}
	if hasMore {
		result["page_token"] = token
	}
	return result, nil
}
