package types

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// LocalizedString represents multi-language text stored as JSONB
// Example: {"en_US": "OpenAI", "zh_Hans": "OpenAI protocol"}
type LocalizedString map[string]string

// Scan implements the sql.Scanner interface
func (l *LocalizedString) Scan(value interface{}) error {
	if value == nil {
		*l = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("failed to scan LocalizedString: expected []byte or string, got %T", value)
	}
	return json.Unmarshal(bytes, l)
}

// Value implements the driver.Valuer interface
func (l LocalizedString) Value() (driver.Value, error) {
	if l == nil {
		return "{}", nil
	}
	return json.Marshal(l)
}

// Get returns the localized string for the given locale
// Falls back to en_US if the locale is not found
func (l LocalizedString) Get(locale string) string {
	if l == nil {
		return ""
	}
	if val, ok := l[locale]; ok {
		return val
	}
	// Fallback to en_US
	if val, ok := l["en_US"]; ok {
		return val
	}
	// Return first available value
	for _, val := range l {
		return val
	}
	return ""
}

// Set sets the localized string for the given locale
func (l *LocalizedString) Set(locale, value string) {
	if *l == nil {
		*l = make(LocalizedString)
	}
	(*l)[locale] = value
}

// Supported locales
const (
	LocaleEnUS   = "en_US"
	LocaleZhHans = "zh_Hans"
	LocaleZhHant = "zh_Hant"
	LocaleJaJP   = "ja_JP"
	LocaleKoKR   = "ko_KR"
)
