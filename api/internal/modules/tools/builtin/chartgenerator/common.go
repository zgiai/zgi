package chartgenerator

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
)

var (
	chartFilenameUnsafePattern = regexp.MustCompile(`[^a-zA-Z0-9._\-\p{Han}]`)
	hexColorPattern            = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

type chartRenderMeta struct {
	ChartType   string
	XCount      int
	SeriesCount int
}

type chartSeries struct {
	Name   string
	Values []float64
	Color  string
}

type chartStyle struct {
	Background string
	Text       string
	MutedText  string
	Axis       string
	Grid       string
	Palette    []string
}

func styleFromOptions(options map[string]interface{}) chartStyle {
	switch strings.ToLower(strings.TrimSpace(stringValue(options["style"]))) {
	case "teaching":
		return chartStyle{
			Background: "#ffffff",
			Text:       "#111827",
			MutedText:  "#4b5563",
			Axis:       "#374151",
			Grid:       "#dbeafe",
			Palette:    []string{"#2563eb", "#f97316", "#16a34a", "#dc2626", "#7c3aed", "#0891b2"},
		}
	case "comparison":
		return chartStyle{
			Background: "#ffffff",
			Text:       "#111827",
			MutedText:  "#4b5563",
			Axis:       "#374151",
			Grid:       "#e5e7eb",
			Palette:    []string{"#94a3b8", "#2563eb", "#16a34a", "#dc2626", "#7c3aed", "#0891b2"},
		}
	case "business":
		return chartStyle{
			Background: "#ffffff",
			Text:       "#0f172a",
			MutedText:  "#475569",
			Axis:       "#334155",
			Grid:       "#e2e8f0",
			Palette:    []string{"#0f766e", "#2563eb", "#9333ea", "#ea580c", "#475569", "#16a34a"},
		}
	default:
		return chartStyle{
			Background: "#ffffff",
			Text:       "#111827",
			MutedText:  "#6b7280",
			Axis:       "#374151",
			Grid:       "#e5e7eb",
			Palette:    []string{"#2563eb", "#94a3b8", "#16a34a", "#dc2626", "#9333ea", "#0891b2"},
		}
	}
}

func defaultColor(index int, style chartStyle) string {
	if index >= 0 && index < len(style.Palette) {
		return style.Palette[index]
	}
	return style.Palette[index%len(style.Palette)]
}

func stringSliceField(data map[string]interface{}, key string) ([]string, error) {
	raw, ok := data[key]
	if !ok {
		return nil, fmt.Errorf("data.%s is required", key)
	}
	items, err := stringSliceValue(raw, "data."+key)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, strings.TrimSpace(item))
	}
	return result, nil
}

func stringSliceValue(raw interface{}, label string) ([]string, error) {
	switch value := raw.(type) {
	case []interface{}:
		result := make([]string, 0, len(value))
		for _, item := range value {
			result = append(result, stringValue(item))
		}
		return result, nil
	case []string:
		return append([]string(nil), value...), nil
	case string:
		text := strings.TrimSpace(value)
		if text == "" {
			return []string{}, nil
		}
		if strings.HasPrefix(text, "[") {
			var values []string
			if err := json.Unmarshal([]byte(text), &values); err != nil {
				return nil, fmt.Errorf("%s must be an array, JSON string array, or CSV string", label)
			}
			return values, nil
		}
		reader := csv.NewReader(strings.NewReader(text))
		reader.TrimLeadingSpace = true
		values, err := reader.Read()
		if err != nil {
			return nil, fmt.Errorf("%s must be an array, JSON string array, or CSV string", label)
		}
		return values, nil
	default:
		return nil, fmt.Errorf("%s must be an array, JSON string array, or CSV string", label)
	}
}

func firstStringSliceField(data map[string]interface{}, keys ...string) ([]string, string, error) {
	var lastErr error
	for _, key := range keys {
		if _, ok := data[key]; !ok {
			continue
		}
		values, err := stringSliceField(data, key)
		if err == nil {
			return values, key, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, "", lastErr
	}
	return nil, "", fmt.Errorf("data.%s is required", keys[0])
}

func numberField(data map[string]interface{}, key string, fallback float64) (float64, error) {
	raw, ok := data[key]
	if !ok || raw == nil {
		return fallback, nil
	}
	return numberValue(raw, "data."+key)
}

func optionalPositiveNumberField(data map[string]interface{}, key string) (float64, bool, error) {
	raw, ok := data[key]
	if !ok || raw == nil {
		return 0, false, nil
	}
	value, err := numberValue(raw, "data."+key)
	if err != nil {
		return 0, false, err
	}
	if value <= 0 {
		return 0, false, fmt.Errorf("data.%s must be greater than 0", key)
	}
	return value, true, nil
}

func optionalNumberField(data map[string]interface{}, key string) (float64, bool, error) {
	raw, ok := data[key]
	if !ok || raw == nil {
		return 0, false, nil
	}
	value, err := numberValue(raw, "data."+key)
	if err != nil {
		return 0, false, err
	}
	return value, true, nil
}

func seriesField(data map[string]interface{}, key string, valueCount int, style chartStyle, maxValue float64, enforceMax bool) ([]chartSeries, error) {
	raw, ok := data[key]
	if !ok {
		return nil, fmt.Errorf("data.%s is required", key)
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("data.%s must be an array", key)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("data.%s must contain at least 1 series", key)
	}
	if len(items) > 8 {
		return nil, fmt.Errorf("data.%s must contain no more than 8 series", key)
	}

	series := make([]chartSeries, 0, len(items))
	for seriesIndex, rawItem := range items {
		item, ok := rawItem.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("data.series[%d] must be an object", seriesIndex)
		}
		name := strings.TrimSpace(stringValue(item["name"]))
		if name == "" {
			name = fmt.Sprintf("Series %d", seriesIndex+1)
		}
		values, err := numberSliceValue(item["values"], fmt.Sprintf("data.series[%d].values", seriesIndex))
		if err != nil {
			return nil, err
		}
		if len(values) != valueCount {
			return nil, fmt.Errorf("data.series[%d].values must contain %d numbers", seriesIndex, valueCount)
		}
		for valueIndex, value := range values {
			if value < 0 {
				return nil, fmt.Errorf("data.series[%d].values[%d] must be greater than or equal to 0", seriesIndex, valueIndex)
			}
			if enforceMax && value > maxValue {
				return nil, fmt.Errorf("data.series[%d].values[%d] must be between 0 and %s", seriesIndex, valueIndex, formatFloat(maxValue))
			}
		}
		series = append(series, chartSeries{
			Name:   name,
			Values: values,
			Color:  normalizeChartColor(stringValue(item["color"]), defaultColor(seriesIndex, style)),
		})
	}
	return series, nil
}

func numberSliceValue(raw interface{}, label string) ([]float64, error) {
	items, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("%s must be an array", label)
	}
	result := make([]float64, 0, len(items))
	for index, item := range items {
		value, err := numberValue(item, fmt.Sprintf("%s[%d]", label, index))
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func numberValue(raw interface{}, label string) (float64, error) {
	switch value := raw.(type) {
	case float64:
		return value, nil
	case float32:
		return float64(value), nil
	case int:
		return float64(value), nil
	case int64:
		return float64(value), nil
	case json.Number:
		parsed, err := value.Float64()
		if err != nil {
			return 0, fmt.Errorf("%s must be a number", label)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("%s must be a number", label)
	}
}

func maxSeriesValue(series []chartSeries) float64 {
	maxValue := 0.0
	for _, item := range series {
		for _, value := range item.Values {
			if value > maxValue {
				maxValue = value
			}
		}
	}
	if maxValue <= 0 {
		return 1
	}
	return maxValue
}

func chartMaxValue(data map[string]interface{}, series []chartSeries) (float64, error) {
	if value, ok, err := optionalPositiveNumberField(data, "max_value"); err != nil || ok {
		return value, err
	}
	return niceMax(maxSeriesValue(series)), nil
}

func niceMax(value float64) float64 {
	if value <= 0 {
		return 1
	}
	value *= 1.1
	power := math.Pow(10, math.Floor(math.Log10(value)))
	scaled := value / power
	switch {
	case scaled <= 2:
		return 2 * power
	case scaled <= 5:
		return 5 * power
	default:
		return 10 * power
	}
}

func intOption(options map[string]interface{}, key string, fallback, minimum, maximum int) int {
	value, err := numberField(options, key, float64(fallback))
	if err != nil {
		return fallback
	}
	n := int(value)
	if n < minimum {
		return minimum
	}
	if n > maximum {
		return maximum
	}
	return n
}

func boolOption(options map[string]interface{}, key string, fallback bool) bool {
	raw, ok := options[key]
	if !ok || raw == nil {
		return fallback
	}
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true", "1", "yes", "y":
			return true
		case "false", "0", "no", "n":
			return false
		}
	}
	return fallback
}

func chartMapParam(params map[string]interface{}, key string) (map[string]interface{}, error) {
	raw, ok := params[key]
	if !ok || raw == nil {
		return nil, fmt.Errorf("%s is required", key)
	}
	value, ok := raw.(map[string]interface{})
	if ok {
		return value, nil
	}
	text, ok := raw.(string)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", key)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		return nil, fmt.Errorf("%s must be an object or JSON object string", key)
	}
	return decoded, nil
}

func optionalChartMapParam(params map[string]interface{}, key string) (map[string]interface{}, error) {
	raw, ok := params[key]
	if !ok || raw == nil {
		return map[string]interface{}{}, nil
	}
	return chartMapParam(params, key)
}

func chartRawStringParam(params map[string]interface{}, key string) string {
	if params == nil {
		return ""
	}
	value, ok := params[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(stringValue(value))
}

func stringValue(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return value
	default:
		return fmt.Sprint(value)
	}
}

func normalizeChartColor(value, fallback string) string {
	text := strings.TrimSpace(value)
	if hexColorPattern.MatchString(text) {
		return strings.ToLower(text)
	}
	return fallback
}

func formatFloat(value float64) string {
	if math.Abs(value-math.Round(value)) < 1e-9 {
		return fmt.Sprintf("%.0f", math.Round(value))
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", value), "0"), ".")
}

func svgEsc(value string) string {
	return html.EscapeString(value)
}

func resolveChartFileLifecycle(raw string) (tool_file.ToolFileLifecycle, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "persistent":
		return tool_file.ToolFileLifecyclePersistent, nil
	case "temporary":
		return tool_file.ToolFileLifecycleTemporary, nil
	default:
		return "", fmt.Errorf("unsupported lifecycle: %s", raw)
	}
}

func buildChartFilename(raw string, extension string) string {
	name := sanitizeChartFilename(raw)
	if name == "" {
		name = defaultChartFilename
	}
	currentExt := filepath.Ext(name)
	if currentExt != "" {
		name = strings.TrimSuffix(name, currentExt)
	}
	return name + extension
}

func sanitizeChartFilename(raw string) string {
	name := strings.TrimSpace(filepath.Base(raw))
	if name == "." || name == string(filepath.Separator) {
		return ""
	}
	name = chartFilenameUnsafePattern.ReplaceAllString(name, "_")
	name = strings.Trim(name, "._- ")
	if len(name) > 120 {
		name = name[:120]
	}
	return name
}

func appendChartDownloadQuery(rawURL string) string {
	if strings.Contains(rawURL, "?") {
		return rawURL + "&download=1"
	}
	return rawURL + "?download=1"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
