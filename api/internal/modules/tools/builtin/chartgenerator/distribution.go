package chartgenerator

import (
	"fmt"
	"strings"
)

type scoreBand struct {
	Label    string
	Min      float64
	Max      float64
	Count    float64
	HasCount bool
}

func renderScoreDistributionChart(title string, data map[string]interface{}, options map[string]interface{}) (string, chartRenderMeta, error) {
	bands, err := scoreBandsField(data)
	if err != nil {
		return "", chartRenderMeta{}, err
	}
	if !scoreBandsHaveCounts(bands) {
		scores, err := scoreValuesField(data)
		if err != nil {
			return "", chartRenderMeta{}, err
		}
		bands = countScoresByBand(scores, bands)
	}

	categories := make([]interface{}, 0, len(bands))
	values := make([]interface{}, 0, len(bands))
	for _, band := range bands {
		categories = append(categories, band.Label)
		values = append(values, band.Count)
	}
	barData := map[string]interface{}{
		"categories": categories,
		"series": []interface{}{
			map[string]interface{}{"name": "Count", "values": values},
		},
	}
	if maxValue, ok, err := optionalPositiveNumberField(data, "max_value"); err != nil || ok {
		if err != nil {
			return "", chartRenderMeta{}, err
		}
		barData["max_value"] = maxValue
	}
	if strings.TrimSpace(title) == "" {
		title = "Score Distribution"
	}
	svg, meta, err := renderBarChart(title, barData, options)
	if err != nil {
		return "", chartRenderMeta{}, err
	}
	meta.ChartType = "score_distribution"
	return svg, meta, nil
}

func scoreBandsField(data map[string]interface{}) ([]scoreBand, error) {
	raw, ok := data["bands"]
	if !ok {
		return nil, fmt.Errorf("data.bands is required")
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("data.bands must be an array")
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("data.bands must contain at least 1 item")
	}
	if len(items) > 30 {
		return nil, fmt.Errorf("data.bands must contain no more than 30 items")
	}

	bands := make([]scoreBand, 0, len(items))
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("data.bands[%d] must be an object", index)
		}
		label := strings.TrimSpace(stringValue(item["label"]))
		if label == "" {
			label = fmt.Sprintf("Band %d", index+1)
		}
		band := scoreBand{Label: label}
		if value, ok, err := optionalNumberFromMap(item, "count", fmt.Sprintf("data.bands[%d].count", index)); err != nil || ok {
			if err != nil {
				return nil, err
			}
			if value < 0 {
				return nil, fmt.Errorf("data.bands[%d].count must be greater than or equal to 0", index)
			}
			band.Count = value
			band.HasCount = true
		} else {
			minValue, minErr := numberValue(item["min"], fmt.Sprintf("data.bands[%d].min", index))
			if minErr != nil {
				return nil, minErr
			}
			maxValue, maxErr := numberValue(item["max"], fmt.Sprintf("data.bands[%d].max", index))
			if maxErr != nil {
				return nil, maxErr
			}
			if maxValue < minValue {
				return nil, fmt.Errorf("data.bands[%d].max must be greater than or equal to min", index)
			}
			band.Min = minValue
			band.Max = maxValue
		}
		bands = append(bands, band)
	}
	if len(bands) > 0 {
		wantCountMode := bands[0].HasCount
		for index, band := range bands[1:] {
			if band.HasCount != wantCountMode {
				return nil, fmt.Errorf("data.bands[%d] must use the same shape as other bands: either all count or all min/max", index+1)
			}
		}
	}
	return bands, nil
}

func scoreBandsHaveCounts(bands []scoreBand) bool {
	for _, band := range bands {
		if !band.HasCount {
			return false
		}
	}
	return true
}

func scoreValuesField(data map[string]interface{}) ([]float64, error) {
	raw, ok := data["scores"]
	if !ok {
		return nil, fmt.Errorf("data.scores is required when bands do not include count")
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("data.scores must be an array")
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("data.scores must contain at least 1 item")
	}
	scores := make([]float64, 0, len(items))
	for index, rawItem := range items {
		switch item := rawItem.(type) {
		case map[string]interface{}:
			value, err := numberValue(item["value"], fmt.Sprintf("data.scores[%d].value", index))
			if err != nil {
				return nil, err
			}
			scores = append(scores, value)
		default:
			value, err := numberValue(rawItem, fmt.Sprintf("data.scores[%d]", index))
			if err != nil {
				return nil, err
			}
			scores = append(scores, value)
		}
	}
	return scores, nil
}

func countScoresByBand(scores []float64, bands []scoreBand) []scoreBand {
	counted := make([]scoreBand, len(bands))
	copy(counted, bands)
	for _, score := range scores {
		for index := range counted {
			if score >= counted[index].Min && score <= counted[index].Max {
				counted[index].Count += 1
				break
			}
		}
	}
	return counted
}

func optionalNumberFromMap(data map[string]interface{}, key string, label string) (float64, bool, error) {
	raw, ok := data[key]
	if !ok || raw == nil {
		return 0, false, nil
	}
	value, err := numberValue(raw, label)
	if err != nil {
		return 0, false, err
	}
	return value, true, nil
}
