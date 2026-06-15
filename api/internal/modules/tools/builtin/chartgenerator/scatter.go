package chartgenerator

import (
	"fmt"
	"math"
	"strings"
)

type scatterPoint struct {
	X     float64
	Y     float64
	Label string
	Color string
}

func renderScatterChart(title string, data map[string]interface{}, options map[string]interface{}) (string, chartRenderMeta, error) {
	style := styleFromOptions(options)
	points, err := scatterPointsField(data, style)
	if err != nil {
		return "", chartRenderMeta{}, err
	}
	xMin, xMax, yMin, yMax, err := scatterBounds(data, points)
	if err != nil {
		return "", chartRenderMeta{}, err
	}

	width := intOption(options, "width", 900, 480, 1600)
	height := intOption(options, "height", 620, 420, 1200)
	showLabels := boolOption(options, "show_labels", true)
	showGrid := boolOption(options, "grid", true)
	if strings.TrimSpace(title) == "" {
		title = "Scatter Chart"
	}
	xLabel := strings.TrimSpace(stringValue(data["x_label"]))
	yLabel := strings.TrimSpace(stringValue(data["y_label"]))

	frame := newChartFrame(width, height)
	elements := renderChartHeader(title, width, style)
	elements = append(elements, renderScatterGrid(frame, xMin, xMax, yMin, yMax, style, showGrid)...)
	if xLabel != "" {
		elements = append(elements, fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" font-family="Arial, Microsoft YaHei, sans-serif" font-size="13" fill="%s">%s</text>`, frame.left+frame.plotWidth/2, frame.bottom+58, style.MutedText, svgEsc(xLabel)))
	}
	if yLabel != "" {
		elements = append(elements, fmt.Sprintf(`<text x="%.1f" y="%.1f" transform="rotate(-90 %.1f %.1f)" text-anchor="middle" font-family="Arial, Microsoft YaHei, sans-serif" font-size="13" fill="%s">%s</text>`, 20.0, frame.top+frame.plotHeight/2, 20.0, frame.top+frame.plotHeight/2, style.MutedText, svgEsc(yLabel)))
	}

	xAt := func(value float64) float64 {
		if xMax == xMin {
			return frame.left + frame.plotWidth/2
		}
		return frame.left + (value-xMin)/(xMax-xMin)*frame.plotWidth
	}
	yAt := func(value float64) float64 {
		if yMax == yMin {
			return frame.bottom - frame.plotHeight/2
		}
		return frame.bottom - (value-yMin)/(yMax-yMin)*frame.plotHeight
	}
	for _, point := range points {
		x := xAt(point.X)
		y := yAt(point.Y)
		elements = append(elements, fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="5" fill="%s" stroke="#ffffff" stroke-width="1.5"/>`, x, y, point.Color))
		if showLabels && point.Label != "" {
			elements = append(elements, fmt.Sprintf(`<text x="%.1f" y="%.1f" font-family="Arial, Microsoft YaHei, sans-serif" font-size="11" fill="%s">%s</text>`, x+8, y-8, style.Text, svgEsc(point.Label)))
		}
	}

	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img">%s</svg>`,
		width, height, width, height, strings.Join(elements, "\n"))
	return svg, chartRenderMeta{ChartType: "scatter", XCount: len(points), SeriesCount: 1}, nil
}

func scatterPointsField(data map[string]interface{}, style chartStyle) ([]scatterPoint, error) {
	raw, ok := data["points"]
	if !ok {
		return nil, fmt.Errorf("data.points is required")
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("data.points must be an array")
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("data.points must contain at least 1 item")
	}
	if len(items) > 500 {
		return nil, fmt.Errorf("data.points must contain no more than 500 items")
	}

	points := make([]scatterPoint, 0, len(items))
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("data.points[%d] must be an object", index)
		}
		x, err := numberValue(item["x"], fmt.Sprintf("data.points[%d].x", index))
		if err != nil {
			return nil, err
		}
		y, err := numberValue(item["y"], fmt.Sprintf("data.points[%d].y", index))
		if err != nil {
			return nil, err
		}
		points = append(points, scatterPoint{
			X:     x,
			Y:     y,
			Label: strings.TrimSpace(stringValue(item["label"])),
			Color: normalizeChartColor(stringValue(item["color"]), defaultColor(index, style)),
		})
	}
	return points, nil
}

func scatterBounds(data map[string]interface{}, points []scatterPoint) (float64, float64, float64, float64, error) {
	xMin := points[0].X
	xMax := points[0].X
	yMin := points[0].Y
	yMax := points[0].Y
	for _, point := range points[1:] {
		xMin = math.Min(xMin, point.X)
		xMax = math.Max(xMax, point.X)
		yMin = math.Min(yMin, point.Y)
		yMax = math.Max(yMax, point.Y)
	}
	xMinExplicit := false
	xMaxExplicit := false
	yMinExplicit := false
	yMaxExplicit := false
	if value, ok, valueErr := optionalNumberField(data, "x_min"); valueErr != nil || ok {
		if valueErr != nil {
			return 0, 0, 0, 0, valueErr
		}
		xMin = value
		xMinExplicit = true
	}
	if value, ok, valueErr := optionalNumberField(data, "x_max"); valueErr != nil || ok {
		if valueErr != nil {
			return 0, 0, 0, 0, valueErr
		}
		xMax = value
		xMaxExplicit = true
	}
	if value, ok, valueErr := optionalNumberField(data, "y_min"); valueErr != nil || ok {
		if valueErr != nil {
			return 0, 0, 0, 0, valueErr
		}
		yMin = value
		yMinExplicit = true
	}
	if value, ok, valueErr := optionalNumberField(data, "y_max"); valueErr != nil || ok {
		if valueErr != nil {
			return 0, 0, 0, 0, valueErr
		}
		yMax = value
		yMaxExplicit = true
	}
	if xMax <= xMin {
		if xMinExplicit || xMaxExplicit {
			return 0, 0, 0, 0, fmt.Errorf("data.x_max must be greater than data.x_min")
		}
		xMin, xMax = paddedEqualBounds(xMin)
	}
	if yMax <= yMin {
		if yMinExplicit || yMaxExplicit {
			return 0, 0, 0, 0, fmt.Errorf("data.y_max must be greater than data.y_min")
		}
		yMin, yMax = paddedEqualBounds(yMin)
	}
	return xMin, xMax, yMin, yMax, nil
}

func paddedEqualBounds(value float64) (float64, float64) {
	padding := math.Abs(value) * 0.1
	if padding < 1 {
		padding = 1
	}
	return value - padding, value + padding
}

func renderScatterGrid(frame chartFrame, xMin, xMax, yMin, yMax float64, style chartStyle, showGrid bool) []string {
	var elements []string
	for level := 0; level <= 5; level++ {
		xValue := xMin + (xMax-xMin)*float64(level)/5
		yValue := yMin + (yMax-yMin)*float64(level)/5
		x := frame.left + frame.plotWidth*float64(level)/5
		y := frame.bottom - frame.plotHeight*float64(level)/5
		if showGrid {
			elements = append(elements,
				fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1"/>`, x, frame.top, x, frame.bottom, style.Grid),
				fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1"/>`, frame.left, y, frame.right, y, style.Grid),
			)
		}
		elements = append(elements,
			fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" font-family="Arial, sans-serif" font-size="11" fill="%s">%s</text>`, x, frame.bottom+24, style.MutedText, svgEsc(formatFloat(xValue))),
			fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="end" font-family="Arial, sans-serif" font-size="11" fill="%s">%s</text>`, frame.left-8, y+4, style.MutedText, svgEsc(formatFloat(yValue))),
		)
	}
	elements = append(elements,
		fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1.5"/>`, frame.left, frame.bottom, frame.right, frame.bottom, style.Axis),
		fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1.5"/>`, frame.left, frame.top, frame.left, frame.bottom, style.Axis),
	)
	return elements
}
