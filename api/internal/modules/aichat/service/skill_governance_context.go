package service

import (
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/assetresolver"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
)

const (
	skillToolGovernanceRuntimeKey        = "tool_governance"
	skillToolGovernancePermissionKey     = "tool_governance_permission_tier"
	skillToolGovernanceAssetsKey         = "tool_governance_assets"
	defaultSkillGovernanceCandidateLimit = 8
)

func applySkillToolGovernanceRuntimeParameters(params map[string]interface{}, prepared *PreparedChat) map[string]interface{} {
	if params == nil {
		params = map[string]interface{}{}
	}
	governance := map[string]interface{}{}
	if existing, ok := params[skillToolGovernanceRuntimeKey].(map[string]interface{}); ok {
		governance = copyStringAnyMap(existing)
	}
	if governance == nil {
		governance = map[string]interface{}{}
	}
	if strings.TrimSpace(stringFromAny(governance["permission_tier"])) == "" {
		if tier := strings.TrimSpace(stringFromAny(params[skillToolGovernancePermissionKey])); tier != "" {
			governance["permission_tier"] = tier
		} else {
			governance["permission_tier"] = string(toolgovernance.PermissionTierBasic)
		}
	}
	if _, exists := governance["assets"]; !exists {
		if _, flatAssetsExist := params[skillToolGovernanceAssetsKey]; !flatAssetsExist {
			if assets := skillToolGovernanceAssetsFromPrepared(prepared); len(assets) > 0 {
				governance["assets"] = assets
			}
		}
	}
	params[skillToolGovernanceRuntimeKey] = governance
	return params
}

func skillToolGovernanceAssetsFromPrepared(prepared *PreparedChat) []map[string]interface{} {
	if prepared == nil || prepared.parts == nil {
		return nil
	}
	selectors := skillToolGovernanceSelectorsFromQuery(prepared.parts.Query)
	if len(selectors) == 0 {
		selectors = []assetresolver.Selector{{Type: assetresolver.AssetTypeFile}}
	}
	result := assetresolver.Resolve(assetresolver.Request{
		OperationContext:           prepared.parts.RawOperationContext,
		NormalizedOperationContext: prepared.parts.OperationContext,
		Selectors:                  selectors,
		CandidateLimit:             defaultSkillGovernanceCandidateLimit,
	})
	if !allAssetResolutionsResolved(result.Resolutions) {
		return nil
	}
	return toolGovernanceAssetMapsFromAssets(result.Assets)
}

func skillToolGovernanceSelectorsFromQuery(query string) []assetresolver.Selector {
	ordinal, last, ok := fileOrdinalFromQuery(query)
	if !ok {
		return nil
	}
	selector := assetresolver.Selector{Type: assetresolver.AssetTypeFile}
	if last {
		selector.OrdinalText = "last"
	} else {
		selector.Ordinal = ordinal
	}
	if fileType := fileFormatFromQuery(query); fileType != "" {
		if fileType == "pdf" {
			selector.Extension = "pdf"
		} else {
			selector.FileType = fileType
		}
	}
	return []assetresolver.Selector{selector}
}

func allAssetResolutionsResolved(resolutions []assetresolver.Resolution) bool {
	if len(resolutions) == 0 {
		return false
	}
	for _, resolution := range resolutions {
		if resolution.Status != assetresolver.StatusResolved {
			return false
		}
	}
	return true
}

func toolGovernanceAssetMapsFromAssets(assets []assetresolver.Asset) []map[string]interface{} {
	if len(assets) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]map[string]interface{}, 0, len(assets))
	for _, asset := range assets {
		id := strings.TrimSpace(asset.ID)
		name := strings.TrimSpace(asset.Name)
		if id == "" && name == "" {
			continue
		}
		assetType := strings.TrimSpace(asset.Type)
		if assetType == "" {
			assetType = assetresolver.AssetTypeFile
		}
		key := assetType + ":" + id + ":" + name
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		item := map[string]interface{}{"type": assetType}
		if id != "" {
			item["id"] = id
		}
		if name != "" {
			item["name"] = name
		}
		if workspaceID := strings.TrimSpace(asset.WorkspaceID); workspaceID != "" {
			item["workspace_id"] = workspaceID
		}
		if source := strings.TrimSpace(asset.Source); source != "" {
			item["source"] = source
		}
		if metadata := copyStringAnyMap(asset.Metadata); len(metadata) > 0 {
			item["metadata"] = metadata
		}
		out = append(out, item)
	}
	return out
}

func fileOrdinalFromQuery(query string) (int, bool, bool) {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return 0, false, false
	}
	if strings.Contains(text, "last") || strings.Contains(text, "\u6700\u540e") {
		return 0, true, true
	}
	if ordinal, ok := chineseOrdinalFromText(text); ok {
		return ordinal, false, true
	}
	if ordinal, ok := englishOrdinalFromText(text); ok {
		return ordinal, false, true
	}
	return 0, false, false
}

func chineseOrdinalFromText(text string) (int, bool) {
	start := strings.Index(text, "\u7b2c")
	if start < 0 {
		return 0, false
	}
	after := text[start+len("\u7b2c"):]
	end := strings.Index(after, "\u4e2a")
	if end < 0 {
		end = strings.Index(after, "\u4efd")
	}
	if end < 0 {
		end = strings.Index(after, "\u5f20")
	}
	if end < 0 {
		return 0, false
	}
	return parseChineseOrdinalToken(after[:end])
}

func parseChineseOrdinalToken(token string) (int, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, false
	}
	if ordinal, err := strconv.Atoi(token); err == nil && ordinal > 0 {
		return ordinal, true
	}
	digit := map[rune]int{
		'\u96f6': 0,
		'\u4e00': 1,
		'\u4e8c': 2,
		'\u4e24': 2,
		'\u4e09': 3,
		'\u56db': 4,
		'\u4e94': 5,
		'\u516d': 6,
		'\u4e03': 7,
		'\u516b': 8,
		'\u4e5d': 9,
	}
	runes := []rune(token)
	if len(runes) == 1 {
		value, ok := digit[runes[0]]
		return value, ok && value > 0
	}
	if len(runes) == 2 && runes[0] == '\u5341' {
		value, ok := digit[runes[1]]
		return 10 + value, ok
	}
	if len(runes) == 2 && runes[1] == '\u5341' {
		value, ok := digit[runes[0]]
		return value * 10, ok && value > 0
	}
	if len(runes) == 3 && runes[1] == '\u5341' {
		tens, tensOK := digit[runes[0]]
		ones, onesOK := digit[runes[2]]
		if tensOK && onesOK && tens > 0 {
			return tens*10 + ones, true
		}
	}
	return 0, false
}

func englishOrdinalFromText(text string) (int, bool) {
	for _, token := range strings.Fields(text) {
		token = strings.Trim(token, ".,;:!?()[]{}")
		if token == "" {
			continue
		}
		if ordinal, ok := parseEnglishOrdinalToken(token); ok {
			return ordinal, true
		}
	}
	return 0, false
}

func parseEnglishOrdinalToken(token string) (int, bool) {
	token = strings.ToLower(strings.TrimSpace(token))
	switch token {
	case "first":
		return 1, true
	case "second":
		return 2, true
	case "third":
		return 3, true
	case "fourth":
		return 4, true
	case "fifth":
		return 5, true
	}
	for _, suffix := range []string{"st", "nd", "rd", "th"} {
		if strings.HasSuffix(token, suffix) {
			value := strings.TrimSuffix(token, suffix)
			ordinal, err := strconv.Atoi(value)
			return ordinal, err == nil && ordinal > 0
		}
	}
	return 0, false
}

func fileFormatFromQuery(query string) string {
	text := strings.ToLower(strings.TrimSpace(query))
	switch {
	case strings.Contains(text, "excel") || strings.Contains(text, "xlsx") || strings.Contains(text, "xls"):
		return "excel"
	case strings.Contains(text, "pdf"):
		return "pdf"
	default:
		return ""
	}
}
