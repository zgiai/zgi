package assetresolver

import (
	"strconv"
	"strings"
)

type criteria struct {
	assetType      string
	directID       string
	selectedOnly   bool
	hasOrdinal     bool
	ordinal        int
	ordinalLast    bool
	hasNameMatcher bool
	titleContains  string
	nameContains   string
	fuzzyName      string
	extensions     []string
	mimeTypes      []string
	fileTypes      []string
}

func criteriaFromSelector(selector Selector) criteria {
	metadataType := stringValue(firstMapValue(selector.Metadata, "type", "resource_type", "kind"))
	out := criteria{
		assetType: normalizeAssetKind(firstNonEmptyString(selector.ResourceType, selector.Kind, stringValue(selector.Metadata["resource_type"]), stringValue(selector.Metadata["kind"]))),
	}
	if out.assetType == "" {
		if kind := normalizeAssetKind(selector.Type); kind == AssetTypeFile || kind == "agent" || kind == "workflow" {
			out.assetType = kind
		}
	}
	if out.assetType == "" {
		if kind := normalizeAssetKind(metadataType); kind == AssetTypeFile || kind == "agent" || kind == "workflow" {
			out.assetType = kind
		}
	}
	if out.assetType == "" {
		out.assetType = AssetTypeFile
	}
	if selector.Type != "" && normalizeAssetKind(selector.Type) != AssetTypeFile && out.assetType == AssetTypeFile {
		out.fileTypes = append(out.fileTypes, normalizeToken(selector.Type))
	}
	if metadataType != "" && normalizeAssetKind(metadataType) != AssetTypeFile && out.assetType == AssetTypeFile {
		out.fileTypes = append(out.fileTypes, normalizeToken(metadataType))
	}

	out.directID = directIDFromSelector(selector)
	out.selectedOnly = selector.Selected ||
		boolValue(selector.Metadata["selected"]) ||
		isSelectedScope(selector.Scope) ||
		isSelectedScope(stringValue(selector.Metadata["scope"]))
	out.titleContains = firstNonEmptyString(selector.TitleContains, stringValue(selector.Metadata["title_contains"]), stringValue(selector.Metadata["contains"]))
	out.nameContains = firstNonEmptyString(selector.NameContains, stringValue(selector.Metadata["name_contains"]), stringValue(selector.Metadata["filename_contains"]))
	out.fuzzyName = firstNonEmptyString(selector.FuzzyName, selector.Title, selector.Name, stringValue(selector.Metadata["fuzzy_name"]), stringValue(selector.Metadata["title"]), stringValue(selector.Metadata["name"]), stringValue(selector.Metadata["filename"]))
	out.hasNameMatcher = strings.TrimSpace(out.titleContains) != "" ||
		strings.TrimSpace(out.nameContains) != "" ||
		strings.TrimSpace(out.fuzzyName) != ""

	out.extensions = append(out.extensions, normalizedExtension(selector.Extension))
	out.extensions = append(out.extensions, normalizedExtensions(selector.Extensions)...)
	out.extensions = append(out.extensions, normalizedExtensions(stringSlice(firstMapValue(selector.Metadata, "extensions", "extension", "ext")))...)
	out.mimeTypes = append(out.mimeTypes, normalizedMimeType(selector.MimeType))
	out.mimeTypes = append(out.mimeTypes, normalizedMimeTypes(selector.MimeTypes)...)
	out.mimeTypes = append(out.mimeTypes, normalizedMimeTypes(stringSlice(firstMapValue(selector.Metadata, "mime_types", "mime_type", "mime", "content_type")))...)
	out.fileTypes = append(out.fileTypes, normalizedFileType(selector.FileType))
	out.fileTypes = append(out.fileTypes, normalizedFileType(stringValue(firstMapValue(selector.Metadata, "file_type", "format", "category"))))
	out.fileTypes = append(out.fileTypes, fileTypesFromReferenceText(selector.Selector)...)
	out.fileTypes = append(out.fileTypes, fileTypesFromReferenceText(selector.Source)...)
	out.fileTypes = append(out.fileTypes, fileTypesFromReferenceText(selector.OrdinalText)...)
	out.fileTypes = append(out.fileTypes, fileTypesFromReferenceText(stringValue(firstMapValue(selector.Metadata, "selector", "reference")))...)
	out.extensions = compactStrings(out.extensions)
	out.mimeTypes = compactStrings(out.mimeTypes)
	out.fileTypes = compactStrings(out.fileTypes)
	out.applyOrdinal(selector)
	return out
}

func (c *criteria) applyOrdinal(selector Selector) {
	for _, value := range []interface{}{
		selector.Ordinal,
		selector.VisibleIndex,
		selector.OrdinalText,
		selector.Selector,
		selector.Source,
		firstMapValue(selector.Metadata, "ordinal", "visible_ordinal", "visible_index", "index", "position", "rank", "selector", "reference"),
	} {
		ordinal, last, ok := parseOrdinal(value)
		if !ok {
			continue
		}
		c.hasOrdinal = true
		c.ordinal = ordinal
		c.ordinalLast = last
		return
	}
	if ordinal, last, ok := parseOrdinal(selector.ID); ok {
		c.hasOrdinal = true
		c.ordinal = ordinal
		c.ordinalLast = last
	}
}

func directIDFromSelector(selector Selector) string {
	for _, value := range []string{
		selector.FileID,
		stringValue(firstMapValue(selector.Metadata, "file_id", "upload_file_id", "resource_id")),
		selector.ID,
	} {
		id := strings.TrimSpace(value)
		if id == "" {
			continue
		}
		if _, _, ok := parseOrdinal(id); ok {
			continue
		}
		return id
	}
	return ""
}

func parseOrdinal(value interface{}) (int, bool, bool) {
	switch typed := value.(type) {
	case int:
		if typed > 0 {
			return typed, false, true
		}
	case int64:
		if typed > 0 {
			return int(typed), false, true
		}
	case float64:
		asInt := int(typed)
		if typed == float64(asInt) && asInt > 0 {
			return asInt, false, true
		}
	case string:
		return parseOrdinalString(typed)
	}
	return 0, false, false
}

func parseOrdinalString(value string) (int, bool, bool) {
	text := strings.ToLower(strings.TrimSpace(value))
	if text == "" {
		return 0, false, false
	}
	if selectorValue, ok := visibleFilesSelectorValue(text); ok {
		return parseOrdinalString(selectorValue)
	}
	switch text {
	case "last", "latest", "final":
		return 0, true, true
	case "first":
		return 1, false, true
	}
	if hasChineseLastOrdinal(text) {
		return 0, true, true
	}
	if hasEnglishLastOrdinal(text) {
		return 0, true, true
	}
	if ordinal, ok := chineseOrdinalFromText(text); ok {
		return ordinal, false, true
	}
	if ordinal, ok := englishOrdinalFromText(text); ok {
		return ordinal, false, true
	}
	ordinal, err := strconv.Atoi(text)
	if err != nil || ordinal <= 0 {
		return 0, false, false
	}
	return ordinal, false, true
}

func visibleFilesSelectorValue(text string) (string, bool) {
	open := strings.Index(text, "[")
	close := strings.Index(text, "]")
	if open <= 0 || close <= open {
		return "", false
	}
	prefix := normalizeKey(text[:open])
	if prefix != "visiblefiles" {
		return "", false
	}
	return strings.TrimSpace(text[open+1 : close]), true
}

func hasChineseLastOrdinal(text string) bool {
	return strings.Contains(text, "\u6700\u540e")
}

func hasEnglishLastOrdinal(text string) bool {
	for _, token := range strings.Fields(text) {
		token = strings.Trim(token, ".,;:!?()[]{}")
		switch token {
		case "last", "latest", "final":
			return true
		}
	}
	return false
}

func chineseOrdinalFromText(text string) (int, bool) {
	start := strings.Index(text, "\u7b2c")
	if start < 0 {
		return 0, false
	}
	after := text[start+len("\u7b2c"):]
	end := -1
	for _, marker := range []string{"\u4e2a", "\u4efd", "\u5f20", "\u9879", "\u6761", "\u4ef6"} {
		index := strings.Index(after, marker)
		if index >= 0 && (end < 0 || index < end) {
			end = index
		}
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
