package wecom

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

var readRoutes = map[string]struct {
	path, kind, items, count, id string
	paged                        bool
}{
	"wecom.documents.search": {"/doc/search", "document", "docs", "docs_count", "docid", true},
	"wecom.documents.get":    {"/doc/contents/get", "document", "", "", "docid", false},
	"wecom.calendar.list":    {"/calendar/schedules/list", "schedule", "schedule_list", "schedule_list_count", "schedule_id", false},
	"wecom.calendar.get":     {"/calendar/schedules/get", "schedule", "schedule_list", "schedule_list_count", "schedule_id", false},
	"wecom.calendar.search":  {"/calendar/schedules/search", "schedule", "schedules", "schedules_count", "schedule_id", true},
	"wecom.todos.list":       {"/todo/list", "todo", "items", "items_count", "todo_id", true},
	"wecom.todos.get":        {"/todo/get", "todo", "items", "items_count", "todo_id", false},
}

func queryHash(op string, body map[string]any) string {
	copy := map[string]any{}
	for k, v := range body {
		if k != "cursor" {
			copy[k] = v
		}
	}
	raw, _ := json.Marshal(struct {
		Operation string
		Query     map[string]any
	}{op, copy})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func stringsInput(value any) []string {
	items, _ := value.([]any)
	result := []string{}
	for _, item := range items {
		v, ok := item.(string)
		if !ok {
			return nil
		}
		result = append(result, v)
	}
	return result
}
func validTimePair(body map[string]any, begin, end string, window bool) bool {
	a, b := providers.Text(body, begin), providers.Text(body, end)
	if a == "" && b == "" {
		return true
	}
	if a == "" || b == "" {
		return false
	}
	const layout = "2006-01-02 15:04:05"
	x, e := time.ParseInLocation(layout, a, time.Local)
	if e != nil {
		return false
	}
	y, e := time.ParseInLocation(layout, b, time.Local)
	if e != nil || !y.After(x) {
		return false
	}
	return !window || (!x.Before(time.Now().AddDate(0, 0, -30)) && !y.After(time.Now().AddDate(0, 0, 30)))
}

func (c *Client) resourceRead(ctx context.Context, op string, in providers.CredentialInput, args map[string]any, token *string) (map[string]any, error) {
	route, ok := readRoutes[op]
	if !ok || c.handles == nil || !validReadScope(in) {
		return nil, providers.Invalid()
	}
	body := map[string]any{}
	for key, value := range args {
		body[key] = value
	}
	for _, keyword := range stringsInput(body["keywords"]) {
		if strings.TrimSpace(keyword) == "" {
			return nil, providers.Invalid()
		}
	}
	if op == "wecom.calendar.search" && len(stringsInput(body["keywords"])) == 0 && providers.Text(body, "organizer") == "" && body["has_attendees"] == nil {
		return nil, providers.Invalid()
	}
	if !validTimePair(body, "begin_time", "end_time", true) || !validTimePair(body, "create_begin_time", "create_end_time", false) || !validTimePair(body, "deadline_begin_time", "deadline_end_time", false) {
		return nil, providers.Invalid()
	}
	if op == "wecom.calendar.list" && body["begin_time"] == nil {
		// The API accepts wall-clock strings only. Preserve response timezone
		// metadata; these explicit default dates use the deployment's timezone.
		now := time.Now()
		body["begin_time"] = now.Format("2006-01-02 15:04:05")
		body["end_time"] = now.AddDate(0, 0, 7).Format("2006-01-02 15:04:05")
	}
	if route.paged && body["limit"] == nil {
		body["limit"] = 10
	}
	query := queryHash(op, body)
	oldCursor := ""
	if cursor := providers.Text(body, "cursor"); cursor != "" {
		var err error
		oldCursor, err = c.openResource(in, cursor, "cursor:"+op, query)
		if err != nil {
			return nil, err
		}
		body["cursor"] = oldCursor
	}
	requested := map[string]string{}
	if op == "wecom.documents.get" {
		id, err := c.openResource(in, providers.Text(body, "resource_handle"), route.kind, "")
		if err != nil {
			return nil, err
		}
		delete(body, "resource_handle")
		body["docid"] = id
		if body["content_type"] == nil {
			body["content_type"] = "markdown"
		}
	} else if strings.HasSuffix(op, ".get") {
		ids := []string{}
		items := []map[string]string{}
		for _, handle := range stringsInput(body["resource_handles"]) {
			id, err := c.openResource(in, handle, route.kind, "")
			if err != nil {
				return nil, err
			}
			if _, duplicate := requested[id]; duplicate {
				return nil, providers.Invalid()
			}
			requested[id] = handle
			ids = append(ids, id)
			items = append(items, map[string]string{"todo_id": id})
		}
		if len(ids) == 0 || len(ids) > 20 {
			return nil, providers.Invalid()
		}
		delete(body, "resource_handles")
		if route.kind == "schedule" {
			body["schedule_ids"] = ids
		} else {
			body["items"] = items
		}
	}
	result, err := c.readBusiness(ctx, route.path, in, token, body)
	if err != nil {
		return nil, err
	}
	if op == "wecom.documents.get" {
		return documentContent(result, providers.Text(body, "content_type"))
	}
	items, ok := result[route.items].([]any)
	if !ok {
		if _, present := result[route.items]; !present && result[route.count] == float64(0) {
			items = []any{}
		} else {
			return nil, responseUnrecognized()
		}
	}
	if len(items) > 200 {
		return nil, responseUnrecognized()
	}
	rows := []any{}
	seen := map[string]bool{}
	for _, item := range items {
		raw, ok := item.(map[string]any)
		id := providers.Text(raw, route.id)
		if !ok || id == "" || seen[id] {
			return nil, responseUnrecognized()
		}
		seen[id] = true
		if len(requested) > 0 && requested[id] == "" {
			return nil, responseUnrecognized()
		}
		handle, err := c.sealResource(in, route.kind, id, "")
		if err != nil {
			return nil, err
		}
		row := map[string]any{"resource_handle": handle}
		if op == "wecom.todos.get" {
			success, ok := raw["success"].(bool)
			if !ok {
				return nil, responseUnrecognized()
			}
			row["success"] = success
			if !success {
				rows = append(rows, row)
				continue
			}
		}
		for _, field := range []struct {
			from, to string
			max      int
		}{{"title", "title", 512}, {"doc_name", "title", 512}, {"subject", "title", 512}, {"description", "description", 4000}, {"doc_type", "type", 64}, {"status", "status", 64}, {"begin_time", "begin_time", 64}, {"end_time", "end_time", 64}} {
			if value, exists := raw[field.from]; exists {
				if text, ok := value.(string); ok {
					row[field.to] = providers.SafeText(text, field.max)
				}
			}
		}
		if zone, ok := raw["timezone"].(map[string]any); ok {
			z := map[string]any{}
			if id, ok := zone["timezone_id"].(string); ok {
				z["timezone_id"] = providers.SafeText(id, 128)
			}
			if offset, ok := zone["timezone_offset"].(float64); ok && offset == float64(int32(offset)) {
				z["timezone_offset"] = offset
			}
			row["timezone"] = z
		}
		if deadline, ok := raw["deadline"].(map[string]any); ok {
			kind := providers.Text(deadline, "type")
			if kind != "date" && kind != "datetime" {
				return nil, responseUnrecognized()
			}
			row["deadline"] = map[string]any{"type": kind, "value": providers.SafeText(providers.Text(deadline, "value"), 64)}
		}
		rows = append(rows, row)
	}
	out := map[string]any{"items": rows}
	if len(requested) > 0 {
		missing := []string{}
		for id, handle := range requested {
			if !seen[id] {
				missing = append(missing, handle)
			}
		}
		out["not_returned"] = missing
	}
	if route.paged {
		more := false
		if v, present := result["has_more"]; present {
			var valid bool
			more, valid = v.(bool)
			if !valid {
				return nil, responseUnrecognized()
			}
		}
		out["has_more"] = more
		if more {
			next := providers.Text(result, "next_cursor")
			if next == "" || next == oldCursor {
				return nil, responseUnrecognized()
			}
			handle, err := c.sealResource(in, "cursor:"+op, next, query)
			if err != nil {
				return nil, err
			}
			out["next_cursor"] = handle
		}
	}
	return out, nil
}

func documentContent(raw map[string]any, format string) (map[string]any, error) {
	content, ok := raw["content"].(string)
	if !ok {
		switch value := raw["file_path"].(type) {
		case string:
			content, ok = value, true
		case map[string]any:
			content, ok = value["content"].(string)
			if !ok {
				return nil, responseUnrecognized()
			}
			encoding := providers.Text(value, "content_encoding")
			if encoding == "base64" {
				decoded, err := base64.StdEncoding.DecodeString(content)
				if err != nil || !utf8.Valid(decoded) {
					return nil, responseUnrecognized()
				}
				content = string(decoded)
				clear(decoded)
			} else if encoding != "" && encoding != "utf8" && encoding != "utf-8" {
				return nil, responseUnrecognized()
			}
		}
	}
	if !ok || !utf8.ValidString(content) {
		return nil, responseUnrecognized()
	}
	truncated := len(content) > 256<<10
	if truncated {
		content = content[:256<<10]
		for !utf8.ValidString(content) {
			content = content[:len(content)-1]
		}
	}
	return map[string]any{"title": providers.SafeText(providers.Text(raw, "name"), 512), "content": content, "content_type": format, "truncated": truncated}, nil
}
