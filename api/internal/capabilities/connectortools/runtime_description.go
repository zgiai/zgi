package connectortools

import (
	"sort"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
)

const operationLocalizedNameBudget = 256

// OperationRuntimeDescription augments an admitted Connector operation's
// English instructions with bounded positive Chinese names for lexical tool
// discovery. It deliberately excludes localized long descriptions: a safety
// phrase such as "does not send messages" is not a sending capability.
// Descriptions are presentation only; callers must still use authorized
// operation snapshots for runtime projection.
func OperationRuntimeDescription(definition catalog.Definition, operation catalog.Operation) string {
	description := strings.TrimSpace(operation.Description)
	if description == "" {
		description = strings.TrimSpace(operation.Name)
	}
	action := simplifiedChineseName(operation.NameI18n)
	connector := simplifiedChineseName(definition.NameI18n)
	if strings.EqualFold(action, connector) {
		connector = ""
	}
	if action == "" && connector == "" {
		return description
	}
	separator := ""
	if description != "" {
		separator = "\n\n"
	}
	remaining := operationLocalizedNameBudget - len([]rune(separator))
	// Allocate the action first so a long connector brand cannot crowd out the
	// business operation. Never truncate the existing English instructions.
	localized := boundedName(action, remaining)
	remaining -= len([]rune(localized))
	if connector != "" {
		joiner := ""
		if localized != "" {
			joiner = " · "
		}
		if remaining > len([]rune(joiner)) {
			localized += joiner + boundedName(connector, remaining-len([]rune(joiner)))
		}
	}
	return description + separator + localized
}

func simplifiedChineseName(values catalog.LocalizedText) string {
	// These are exactly the aliases grouped by the catalog's locale validator.
	// Underscore tags, zh-Hant and arbitrary locale fallbacks are not aliases.
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, alias := range []string{"zh-hans", "zh-cn", "zh"} {
		for _, key := range keys {
			if strings.EqualFold(key, alias) {
				if name := strings.Join(strings.Fields(values[key]), " "); name != "" {
					return name
				}
			}
		}
	}
	return ""
}

func boundedName(value string, limit int) string {
	characters := []rune(value)
	if len(characters) > limit {
		characters = characters[:limit]
	}
	return string(characters)
}
