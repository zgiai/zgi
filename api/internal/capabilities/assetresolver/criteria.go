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
	out.fileTypes = append(out.fileTypes, normalizeToken(selector.FileType))
	out.fileTypes = append(out.fileTypes, normalizeToken(stringValue(firstMapValue(selector.Metadata, "file_type", "format", "category"))))
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
	switch text {
	case "last", "latest", "final":
		return 0, true, true
	case "first":
		return 1, false, true
	}
	if selectorValue, ok := visibleFilesSelectorValue(text); ok {
		return parseOrdinalString(selectorValue)
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
