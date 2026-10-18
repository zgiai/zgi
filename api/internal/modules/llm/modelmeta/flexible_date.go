package modelmeta

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const modelMetaDateFormat = "2006-01-02"

// modelMetaDate holds a date-ish ModelMeta field.
//
// The documented schema sends these fields as strings, but the curated ZGI
// Models service (https://models.zgi.ai) sends Unix timestamps. A single
// unexpected numeric field fails the whole page's JSON decode, which makes the
// provider unsyncable, so both shapes are accepted here and numbers are
// normalized to YYYY-MM-DD.
type modelMetaDate string

// UnmarshalJSON accepts a JSON string, a numeric Unix timestamp, or null.
func (d *modelMetaDate) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*d = ""
		return nil
	}

	if trimmed[0] == '"' {
		var value string
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return err
		}
		*d = modelMetaDate(strings.TrimSpace(value))
		return nil
	}

	var number json.Number
	if err := json.Unmarshal(trimmed, &number); err != nil {
		return fmt.Errorf("model meta date: unsupported value %s", string(trimmed))
	}
	*d = modelMetaDate(normalizeModelMetaDate(number.String()))
	return nil
}

// String returns the normalized date.
func (d modelMetaDate) String() string { return string(d) }

// normalizeModelMetaDate converts a numeric Unix timestamp (seconds or
// milliseconds) to YYYY-MM-DD, and returns any other value trimmed as-is, so
// sources that send strings and sources that send timestamps end up in one
// format that fits the varchar(20) catalog column.
func normalizeModelMetaDate(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	value, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return trimmed
	}

	switch {
	case value >= 1e12:
		return time.UnixMilli(value).UTC().Format(modelMetaDateFormat)
	case value >= 1e9:
		return time.Unix(value, 0).UTC().Format(modelMetaDateFormat)
	default:
		// Not a timestamp we recognise (for example a bare "20241001"): keep it.
		return trimmed
	}
}
