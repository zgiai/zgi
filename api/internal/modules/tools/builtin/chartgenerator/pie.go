package chartgenerator

import (
	"fmt"
	"math"
	"strings"
)

type pieItem struct {
	Label string
	Value float64
	Color string
}

func renderPieChart(title string, data map[string]interface{}, options map[string]interface{}) (string, chartRenderMeta, error) {
	return renderPieLikeChart("pie", title, data, options)
}

func renderDoughnutChart(title string, data map[string]interface{}, options map[string]interface{}) (string, chartRenderMeta, error) {
	return renderPieLikeChart("doughnut", title, data, options)
}

func renderPieLikeChart(chartType string, title string, data map[string]interface{}, options map[string]interface{}) (string, chartRenderMeta, error) {
	style := styleFromOptions(options)
	items, err := pieItemsField(data, style)
	if err != nil {
		return "", chartRenderMeta{}, err
	}
	total := 0.0
	for _, item := range items {
		total += item.Value
	}
	if total <= 0 {
		return "", chartRenderMeta{}, fmt.Errorf("data.items total value must be greater than 0")
	}

	width := intOption(options, "width", 820, 480, 1600)
	height := intOption(options, "height", 620, 420, 1200)
	showValues := boolOption(options, "show_values", true)
	showLegend := boolOption(options, "legend", true)
	if strings.TrimSpace(title) == "" {
		if chartType == "doughnut" {
			title = "Doughnut Chart"
		} else {
			title = "Pie Chart"
		}
	}

	centerX := float64(width) * 0.38
	centerY := float64(height)/2 + 20
	radius := math.Min(float64(width)*0.24, float64(height)*0.32)
	startAngle := -math.Pi / 2

	elements := renderChartHeader(title, width, style)
	for _, item := range items {
		angle := item.Value / total * math.Pi * 2
		endAngle := startAngle + angle
		if len(items) == 1 {
			elements = append(elements, fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s"/>`, centerX, centerY, radius, item.Color))
		} else {
			elements = append(elements, pieSlicePath(centerX, centerY, radius, startAngle, endAngle, item.Color))
		}
		if showValues {
			labelAngle := startAngle + angle/2
			labelX := centerX + math.Cos(labelAngle)*radius*0.64
			labelY := centerY + math.Sin(labelAngle)*radius*0.64
			percent := item.Value / total * 100
			elements = append(elements, fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" dominant-baseline="middle" font-family="Arial, Microsoft YaHei, sans-serif" font-size="12" font-weight="700" fill="#ffffff">%s%%</text>`, labelX, labelY, svgEsc(formatFloat(percent))))
		}
		startAngle = endAngle
	}
	if chartType == "doughnut" {
		innerRadius := radius * 0.52
		elements = append(elements, fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s"/>`, centerX, centerY, innerRadius, style.Background))
		elements = append(elements, fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" dominant-baseline="middle" font-family="Arial, sans-serif" font-size="16" font-weight="700" fill="%s">%s</text>`, centerX, centerY, style.Text, svgEsc(formatFloat(total))))
	}
	if showLegend {
		legendSeries := make([]chartSeries, 0, len(items))
		for _, item := range items {
			legendSeries = append(legendSeries, chartSeries{
				Name:  fmt.Sprintf("%s (%s)", item.Label, formatFloat(item.Value)),
				Color: item.Color,
			})
		}
		elements = append(elements, renderLegend(float64(width)-260, 105, legendSeries, style)...)
	}

	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img">%s</svg>`,
		width, height, width, height, strings.Join(elements, "\n"))
	return svg, chartRenderMeta{ChartType: chartType, XCount: len(items), SeriesCount: 1}, nil
}

func pieItemsField(data map[string]interface{}, style chartStyle) ([]pieItem, error) {
	raw, ok := data["items"]
	if !ok {
		return nil, fmt.Errorf("data.items is required")
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("data.items must be an array")
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("data.items must contain at least 1 item")
	}
	if len(items) > 50 {
		return nil, fmt.Errorf("data.items must contain no more than 50 items")
	}

	result := make([]pieItem, 0, len(items))
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("data.items[%d] must be an object", index)
		}
		label := strings.TrimSpace(stringValue(item["label"]))
		if label == "" {
			label = fmt.Sprintf("Item %d", index+1)
		}
		value, err := numberValue(item["value"], fmt.Sprintf("data.items[%d].value", index))
		if err != nil {
			return nil, err
		}
		if value < 0 {
			return nil, fmt.Errorf("data.items[%d].value must be greater than or equal to 0", index)
		}
		result = append(result, pieItem{
			Label: label,
			Value: value,
			Color: normalizeChartColor(stringValue(item["color"]), defaultColor(index, style)),
		})
	}
	return result, nil
}

func pieSlicePath(cx, cy, radius, startAngle, endAngle float64, color string) string {
	startX := cx + math.Cos(startAngle)*radius
	startY := cy + math.Sin(startAngle)*radius
	endX := cx + math.Cos(endAngle)*radius
	endY := cy + math.Sin(endAngle)*radius
	largeArc := 0
	if endAngle-startAngle > math.Pi {
		largeArc = 1
	}
	return fmt.Sprintf(`<path d="M %.1f %.1f L %.1f %.1f A %.1f %.1f 0 %d 1 %.1f %.1f Z" fill="%s"/>`, cx, cy, startX, startY, radius, radius, largeArc, endX, endY, color)
}
