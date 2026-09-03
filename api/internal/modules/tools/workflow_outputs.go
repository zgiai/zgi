package tools

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	maxWorkflowOutputDepth = 6
	maxWorkflowOutputs     = 96
)

var workflowOutputIDPart = regexp.MustCompile(`[^a-zA-Z0-9_]+`)

// CompileWorkflowOutputs turns a JSON Schema result contract into flat,
// workflow-safe outputs. In particular, an array of objects also publishes
// primitive leaf projections (results[].url -> array[string]). This avoids
// teaching the global variable pool wildcard traversal and keeps existing
// Tool Runtime consumers unchanged.
func CompileWorkflowOutputs(schema map[string]interface{}) []ToolWorkflowOutput {
	if len(schema) == 0 {
		return nil
	}

	compiler := workflowOutputCompiler{seen: make(map[string]int)}
	compiler.walk(schema, nil, nil, false, 0)
	return compiler.outputs
}

type workflowOutputCompiler struct {
	outputs []ToolWorkflowOutput
	seen    map[string]int
}

func (c *workflowOutputCompiler) walk(schema map[string]interface{}, path, labels []string, projected bool, depth int) {
	if len(c.outputs) >= maxWorkflowOutputs || depth > maxWorkflowOutputDepth {
		return
	}

	typeName := schemaType(schema)
	switch typeName {
	case "object":
		if len(path) > 0 && !projected {
			c.add(path, labels, "object")
		}
		properties, _ := schema["properties"].(map[string]interface{})
		keys := make([]string, 0, len(properties))
		for key := range properties {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			child, ok := properties[key].(map[string]interface{})
			if !ok {
				continue
			}
			label := schemaLabel(child, key)
			c.walk(child, appendPath(path, key), appendPath(labels, label), projected, depth+1)
		}

	case "array":
		items, _ := schema["items"].(map[string]interface{})
		itemType := schemaType(items)
		if len(path) > 0 {
			if primitiveWorkflowType(itemType) != "" {
				c.add(path, labels, "array["+primitiveWorkflowType(itemType)+"]")
			} else if itemType == "object" {
				c.add(path, labels, "array[object]")
			}
		}
		if itemType == "object" {
			c.walk(items, appendPath(path, "*"), labels, true, depth+1)
		}

	default:
		primitive := primitiveWorkflowType(typeName)
		if primitive == "" || len(path) == 0 {
			return
		}
		if projected {
			primitive = "array[" + primitive + "]"
		}
		c.add(path, labels, primitive)
	}
}

func (c *workflowOutputCompiler) add(path, labels []string, outputType string) {
	if len(c.outputs) >= maxWorkflowOutputs || outputType == "" {
		return
	}
	idParts := make([]string, 0, len(path))
	for _, part := range path {
		if part == "*" {
			continue
		}
		cleaned := strings.Trim(workflowOutputIDPart.ReplaceAllString(part, "_"), "_")
		if cleaned != "" {
			idParts = append(idParts, strings.ToLower(cleaned))
		}
	}
	if len(idParts) == 0 {
		return
	}
	baseID := "result__" + strings.Join(idParts, "__")
	count := c.seen[baseID]
	c.seen[baseID] = count + 1
	id := baseID
	if count > 0 {
		id = fmt.Sprintf("%s__%d", baseID, count+1)
	}

	label := strings.Join(labels, " / ")
	if label == "" {
		label = strings.Join(idParts, " / ")
	}
	chineseLabel := localizedWorkflowOutputLabel(labels, idParts)
	c.outputs = append(c.outputs, ToolWorkflowOutput{
		ID:   id,
		Path: append([]string(nil), path...),
		Type: outputType,
		Label: I18nText{
			"en_US":   label,
			"zh_Hans": chineseLabel,
		},
	})
}

var workflowOutputChineseLabels = map[string]string{
	"results":       "结果",
	"items":         "项目",
	"url":           "网页地址",
	"urls":          "网页地址",
	"title":         "标题",
	"text":          "正文",
	"content":       "内容",
	"id":            "标识",
	"name":          "名称",
	"email":         "邮箱",
	"files":         "文件",
	"messages":      "消息",
	"message id":    "消息标识",
	"message ref":   "邮件引用",
	"recipient ref": "接收人引用",
	"user id":       "用户标识",
	"chat id":       "群聊标识",
	"calendar id":   "日历标识",
	"document id":   "文档标识",
	"token":         "资源标识",
	"number":        "编号",
	"owner":         "所有者",
	"repo":          "仓库",
	"full name":     "完整名称",
	"status":        "状态",
	"author":        "作者",
	"published at":  "发布时间",
	"highlights":    "摘要片段",
}

func localizedWorkflowOutputLabel(labels, fallback []string) string {
	parts := labels
	if len(parts) == 0 {
		parts = fallback
	}
	localized := make([]string, 0, len(parts))
	for _, part := range parts {
		key := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(part, "_", " ")))
		if translated := workflowOutputChineseLabels[key]; translated != "" {
			localized = append(localized, translated)
		} else if strings.TrimSpace(part) != "" {
			localized = append(localized, part)
		}
	}
	return strings.Join(localized, " / ")
}

func schemaType(schema map[string]interface{}) string {
	if schema == nil {
		return ""
	}
	if value, ok := schema["type"].(string); ok {
		return value
	}
	switch values := schema["type"].(type) {
	case []string:
		for _, value := range values {
			if value != "null" {
				return value
			}
		}
	case []interface{}:
		for _, raw := range values {
			if value, ok := raw.(string); ok && value != "null" {
				return value
			}
		}
	}
	if _, ok := schema["properties"].(map[string]interface{}); ok {
		return "object"
	}
	if constant, exists := schema["const"]; exists {
		switch constant.(type) {
		case string:
			return "string"
		case bool:
			return "boolean"
		case float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return "number"
		}
	}
	return ""
}

func primitiveWorkflowType(schemaType string) string {
	switch schemaType {
	case "string":
		return "string"
	case "integer", "number":
		return "number"
	case "boolean":
		return "boolean"
	default:
		return ""
	}
}

func schemaLabel(schema map[string]interface{}, fallback string) string {
	if title, ok := schema["title"].(string); ok && strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title)
	}
	return strings.ReplaceAll(fallback, "_", " ")
}

func appendPath(values []string, value string) []string {
	out := make([]string, 0, len(values)+1)
	out = append(out, values...)
	return append(out, value)
}
