package chartgenerator

import (
	"fmt"
	"math"
	"strings"
)

func renderRadarChart(title string, data map[string]interface{}, options map[string]interface{}) (string, chartRenderMeta, error) {
	style := styleFromOptions(options)
	dimensions, err := stringSliceField(data, "dimensions")
	if err != nil {
		return "", chartRenderMeta{}, err
	}
	if len(dimensions) < 3 {
		return "", chartRenderMeta{}, fmt.Errorf("data.dimensions must contain at least 3 items")
	}
	for _, dimension := range dimensions {
		if strings.TrimSpace(dimension) == "" {
			return "", chartRenderMeta{}, fmt.Errorf("data.dimensions must not contain empty labels")
		}
	}

	maxValue, err := numberField(data, "max_value", 100)
	if err != nil {
		return "", chartRenderMeta{}, err
	}
	if maxValue <= 0 {
		return "", chartRenderMeta{}, fmt.Errorf("data.max_value must be greater than 0")
	}

	series, err := seriesField(data, "series", len(dimensions), style, maxValue, true)
	if err != nil {
		return "", chartRenderMeta{}, err
	}
	if len(series) > 2 {
		return "", chartRenderMeta{}, fmt.Errorf("data.series must contain 1 or 2 series")
	}

	width := intOption(options, "width", 900, 480, 1600)
	height := intOption(options, "height", 700, 420, 1200)
	showValues := boolOption(options, "show_values", true)
	showLegend := boolOption(options, "legend", true)
	if strings.TrimSpace(title) == "" {
		title = "Radar Chart"
	}

	centerX := float64(width) / 2
	centerY := float64(height)/2 + 20
	radius := math.Min(float64(width), float64(height)) * 0.32
	axisCount := len(dimensions)

	axisPoint := func(axisIndex int, scale float64) (float64, float64) {
		angle := -math.Pi/2 + 2*math.Pi*float64(axisIndex)/float64(axisCount)
		scaled := radius * scale
		return centerX + math.Cos(angle)*scaled, centerY + math.Sin(angle)*scaled
	}
	valuePoint := func(axisIndex int, value float64) (float64, float64) {
		return axisPoint(axisIndex, value/maxValue)
	}

	var elements []string
	elements = append(elements,
		fmt.Sprintf(`<rect width="100%%" height="100%%" fill="%s"/>`, style.Background),
		fmt.Sprintf(`<text x="%.1f" y="42" text-anchor="middle" font-family="Arial, sans-serif" font-size="24" font-weight="700" fill="%s">%s</text>`, centerX, style.Text, svgEsc(title)),
	)

	for level := 1; level <= 5; level++ {
		scale := float64(level) / 5
		points := make([]string, 0, axisCount)
		for index := 0; index < axisCount; index++ {
			x, y := axisPoint(index, scale)
			points = append(points, fmt.Sprintf("%.1f,%.1f", x, y))
		}
		elements = append(elements, fmt.Sprintf(`<polygon points="%s" fill="none" stroke="%s" stroke-width="1"/>`, strings.Join(points, " "), style.Grid))
		labelValue := maxValue * scale
		elements = append(elements, fmt.Sprintf(`<text x="%.1f" y="%.1f" font-family="Arial, sans-serif" font-size="11" fill="%s">%s</text>`, centerX+6, centerY-radius*scale+4, style.MutedText, svgEsc(formatFloat(labelValue))))
	}

	for index, label := range dimensions {
		x, y := axisPoint(index, 1)
		elements = append(elements, fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1"/>`, centerX, centerY, x, y, style.Grid))
		labelX, labelY := axisPoint(index, 1.16)
		anchor := "middle"
		if labelX > centerX+12 {
			anchor = "start"
		} else if labelX < centerX-12 {
			anchor = "end"
		}
		elements = append(elements, fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="%s" dominant-baseline="middle" font-family="Arial, Microsoft YaHei, sans-serif" font-size="14" fill="%s">%s</text>`, labelX, labelY, anchor, style.Text, svgEsc(label)))
	}

	for seriesIndex, item := range series {
		points := make([]string, 0, len(item.Values))
		for valueIndex, value := range item.Values {
			x, y := valuePoint(valueIndex, value)
			points = append(points, fmt.Sprintf("%.1f,%.1f", x, y))
		}
		opacity := "0.18"
		strokeWidth := "2.5"
		if seriesIndex == len(series)-1 {
			opacity = "0.24"
			strokeWidth = "3"
		}
		elements = append(elements, fmt.Sprintf(`<polygon points="%s" fill="%s" fill-opacity="%s" stroke="%s" stroke-width="%s"/>`, strings.Join(points, " "), item.Color, opacity, item.Color, strokeWidth))
		for valueIndex, value := range item.Values {
			x, y := valuePoint(valueIndex, value)
			elements = append(elements, fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="4" fill="%s" stroke="#ffffff" stroke-width="1.5"/>`, x, y, item.Color))
			if showValues && seriesIndex == len(series)-1 {
				elements = append(elements, fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" font-family="Arial, sans-serif" font-size="12" font-weight="600" fill="%s">%s</text>`, x, y-9, item.Color, svgEsc(formatFloat(value))))
			}
		}
	}

	if showLegend {
		elements = append(elements, renderLegend(float64(width)-210, 72, series, style)...)
	}

	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img">%s</svg>`,
		width, height, width, height, strings.Join(elements, "\n"))
	return svg, chartRenderMeta{ChartType: "radar", XCount: len(dimensions), SeriesCount: len(series)}, nil
}
