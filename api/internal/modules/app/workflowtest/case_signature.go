package workflowtest

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"
)

const generatedCaseDuplicateMaxAttempts = 3

type generatedCaseSignatureRegistry struct {
	mu         sync.Mutex
	signatures map[string]struct{}
	shapeKeys  map[string]struct{}
	previews   []string
}

func newGeneratedCaseSignatureRegistry(existing []Case) *generatedCaseSignatureRegistry {
	registry := &generatedCaseSignatureRegistry{
		signatures: map[string]struct{}{},
		shapeKeys:  map[string]struct{}{},
		previews:   make([]string, 0, len(existing)),
	}
	for _, item := range existing {
		signature := workflowTestCaseSignatureFromCase(item)
		if signature.key == "" {
			continue
		}
		registry.signatures[signature.key] = struct{}{}
		for _, key := range signature.shapeKeys {
			registry.shapeKeys[key] = struct{}{}
		}
		if signature.preview != "" {
			registry.previews = append(registry.previews, signature.preview)
		}
	}
	return registry
}

func (r *generatedCaseSignatureRegistry) reserve(item GeneratedCase) (workflowTestCaseSignature, bool) {
	signature := workflowTestCaseSignatureFromGenerated(item)
	if signature.key == "" {
		return signature, true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.signatures[signature.key]; exists {
		return signature, false
	}
	for _, key := range signature.shapeKeys {
		if _, exists := r.shapeKeys[key]; exists {
			return signature, false
		}
	}
	r.signatures[signature.key] = struct{}{}
	for _, key := range signature.shapeKeys {
		r.shapeKeys[key] = struct{}{}
	}
	if signature.preview != "" {
		r.previews = append(r.previews, signature.preview)
	}
	return signature, true
}

func (r *generatedCaseSignatureRegistry) avoidancePrompt(limit int) string {
	if r == nil || limit <= 0 {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.previews) == 0 {
		return ""
	}
	start := len(r.previews) - limit
	if start < 0 {
		start = 0
	}
	values := append([]string(nil), r.previews[start:]...)
	return buildDuplicateAvoidancePrompt(values)
}

type workflowTestCaseSignature struct {
	key       string
	shapeKeys []string
	preview   string
}

func workflowTestCaseSignatureFromCase(item Case) workflowTestCaseSignature {
	parts := []string{item.Content}
	parts = append(parts, caseTurnsSignatureParts(item.Turns)...)
	return workflowTestCaseSignatureFromParts(parts)
}

func workflowTestCaseSignatureFromGenerated(item GeneratedCase) workflowTestCaseSignature {
	parts := []string{item.Content}
	parts = append(parts, caseTurnsSignatureParts(item.Turns)...)
	for _, fixture := range item.FileFixtures {
		parts = append(parts,
			fixture.Filename,
			fixture.Format,
			fixture.Title,
			fixture.Content,
			fixture.Description,
		)
		parts = append(parts, fixture.Facts...)
		parts = append(parts, fixture.ExpectedChecks...)
	}
	return workflowTestCaseSignatureFromParts(parts)
}

func caseTurnsSignatureParts(turns []CaseTurn) []string {
	parts := make([]string, 0, len(turns)*2)
	for _, turn := range turns {
		role := strings.ToLower(strings.TrimSpace(turn.Role))
		if role != "" && role != "user" {
			continue
		}
		parts = append(parts, turn.Content)
		parts = append(parts, caseInputsSignatureParts(turn.Inputs)...)
		for _, attachment := range turn.Attachments {
			parts = append(parts, attachment.Name, attachment.URL, attachment.UploadFileID)
		}
	}
	return parts
}

func caseInputsSignatureParts(inputs JSONMap) []string {
	if len(inputs) == 0 {
		return nil
	}
	keys := make([]string, 0, len(inputs))
	for key := range inputs {
		name := strings.TrimSpace(key)
		if name == "" || isWorkflowTestReservedInputKey(name) || strings.HasPrefix(name, "sys.") {
			continue
		}
		keys = append(keys, name)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+signatureValueString(inputs[key]))
	}
	if fixtureValue, ok := inputs[generatedFixtureSpecKey]; ok {
		parts = append(parts, signatureValueString(fixtureValue))
	}
	return parts
}

func workflowTestCaseSignatureFromParts(parts []string) workflowTestCaseSignature {
	normalizedParts := make([]string, 0, len(parts))
	previewParts := make([]string, 0, len(parts))
	seenParts := map[string]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		normalized := normalizeWorkflowTestSignatureText(part)
		if normalized == "" {
			continue
		}
		if _, exists := seenParts[normalized]; exists {
			continue
		}
		seenParts[normalized] = struct{}{}
		normalizedParts = append(normalizedParts, normalized)
		if len(previewParts) < 3 {
			previewParts = append(previewParts, truncateSignaturePreview(part, 80))
		}
	}
	if len(normalizedParts) == 0 {
		return workflowTestCaseSignature{}
	}
	shapeKeys := workflowTestCaseShapeKeys(normalizedParts, previewParts)
	return workflowTestCaseSignature{
		key:       strings.Join(normalizedParts, "|"),
		shapeKeys: shapeKeys,
		preview:   strings.Join(previewParts, " / "),
	}
}

func workflowTestCaseShapeKeys(normalizedParts []string, previewParts []string) []string {
	keys := make([]string, 0, 2)
	if len(normalizedParts) > 0 {
		if key := normalizeWorkflowTestTemplateText(strings.Join(normalizedParts, "|")); key != "" {
			keys = append(keys, "template:"+key)
		}
	}
	if len(normalizedParts) == 1 && len(previewParts) > 0 {
		if key := workflowTestRequestIntentShape(previewParts[0]); key != "" {
			keys = append(keys, "intent:"+key)
		}
	}
	return keys
}

func normalizeWorkflowTestSignatureText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastSpace := false
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			if !lastSpace {
				builder.WriteRune(' ')
				lastSpace = true
			}
			continue
		}
		builder.WriteRune(r)
		lastSpace = false
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func normalizeWorkflowTestTemplateText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastSpace := false
	lastPlaceholder := false
	for _, r := range value {
		if isVolatileSignatureRune(r) {
			if !lastPlaceholder {
				builder.WriteRune('#')
				lastPlaceholder = true
			}
			lastSpace = false
			continue
		}
		lastPlaceholder = false
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			if !lastSpace {
				builder.WriteRune(' ')
				lastSpace = true
			}
			continue
		}
		builder.WriteRune(r)
		lastSpace = false
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func isVolatileSignatureRune(r rune) bool {
	if unicode.IsDigit(r) {
		return true
	}
	return strings.ContainsRune("零一二三四五六七八九十百千万亿两〇年月日号第上下东南西北中高低+-%.％￥$¥", r)
}

func workflowTestRequestIntentShape(value string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}
	candidates := []string{"请", "帮我", "帮忙", "麻烦", "能否", "可以"}
	start := -1
	for _, marker := range candidates {
		if idx := strings.LastIndex(text, marker); idx >= 0 && idx > start {
			start = idx
		}
	}
	if start < 0 {
		return ""
	}
	intent := strings.TrimSpace(text[start:])
	intent = normalizeRequestIntentBoilerplate(intent)
	if len([]rune(intent)) < 8 {
		return ""
	}
	return normalizeWorkflowTestTemplateText(intent)
}

func normalizeRequestIntentBoilerplate(value string) string {
	replacements := []string{
		"基于这个情况",
		"基于这种情况",
		"基于这些数据",
		"基于上述数据",
		"基于以上数据",
		"基于上述情况",
		"基于以上情况",
		"根据这些数据",
		"根据上述数据",
		"根据以上数据",
		"根据这个情况",
		"根据上述情况",
		"根据以上情况",
		"根据以上内容",
		"这个情况",
		"这种情况",
		"这些数据",
		"上述数据",
		"以上数据",
		"上述情况",
		"以上情况",
	}
	result := strings.TrimSpace(value)
	for _, item := range replacements {
		result = strings.ReplaceAll(result, item, "")
	}
	return strings.Join(strings.Fields(result), "")
}

func signatureValueString(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(data)
	}
}

func buildDuplicateAvoidancePrompt(values []string) string {
	if len(values) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("本次生成必须避开以下已存在或刚生成的用户输入/样例，不要只替换数字、金额、日期、地区、产品线或一两个词来重复同一问题模板；请改变测试意图、输入结构、业务对象、变量组合、文件事实、异常类型或多轮追问路径：")
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		builder.WriteString("\n- ")
		builder.WriteString(value)
	}
	return builder.String()
}

func truncateSignaturePreview(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}
