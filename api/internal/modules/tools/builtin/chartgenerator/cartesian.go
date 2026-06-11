package chartgenerator

import (
	"fmt"
	"math"
	"strings"
)

type chartFrame struct {
	width      int
	height     int
	left       float64
	right      float64
	top        float64
	bottom     float64
	plotWidth  float64
	plotHeight float64
}

func newChartFrame(width int, height int) chartFrame {
	left := 72.0
	right := float64(width) - 42
	top := 86.0
	bottom := float64(height) - 86
	return chartFrame{
		width:      width,
		height:     height,
		left:       left,
		right:      right,
		top:        top,
		bottom:     bottom,
		plotWidth:  right - left,
		plotHeight: bottom - top,
	}
}

func renderChartHeader(title string, width int, style chartStyle) []string {
	return []string{
		fmt.Sprintf(`<rect width="100%%" height="100%%" fill="%s"/>`, style.Background),
		fmt.Sprintf(`<text x="%.1f" y="42" text-anchor="middle" font-family="Arial, Microsoft YaHei, sans-serif" font-size="24" font-weight="700" fill="%s">%s</text>`, float64(width)/2, style.Text, svgEsc(title)),
	}
}

func renderCartesianGrid(frame chartFrame, maxValue float64, style chartStyle, showGrid bool) []string {
	var elements []string
	if showGrid {
		for level := 0; level <= 5; level++ {
			value := maxValue * float64(level) / 5
			y := frame.bottom - frame.plotHeight*float64(level)/5
			elements = append(elements,
				fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1"/>`, frame.left, y, frame.right, y, style.Grid),
				fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="end" font-family="Arial, sans-serif" font-size="11" fill="%s">%s</text>`, frame.left-8, y+4, style.MutedText, svgEsc(formatFloat(value))),
			)
		}
	}
	elements = append(elements,
		fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1.5"/>`, frame.left, frame.bottom, frame.right, frame.bottom, style.Axis),
		fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1.5"/>`, frame.left, frame.top, frame.left, frame.bottom, style.Axis),
	)
	return elements
}

func renderCategoryLabels(frame chartFrame, labels []string, style chartStyle) []string {
	var elements []string
	if len(labels) == 0 {
		return elements
	}
	step := frame.plotWidth / float64(len(labels))
	for index, label := range labels {
		x := frame.left + step*(float64(index)+0.5)
		elements = append(elements, fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" font-family="Arial, Microsoft YaHei, sans-serif" font-size="12" fill="%s">%s</text>`, x, frame.bottom+24, style.MutedText, svgEsc(label)))
	}
	return elements
}

func renderLegend(x float64, y float64, series []chartSeries, style chartStyle) []string {
	var elements []string
	for index, item := range series {
		rowY := y + float64(index*26)
		elements = append(elements,
			fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="14" height="14" rx="2" fill="%s"/>`, x, rowY-11, item.Color),
			fmt.Sprintf(`<text x="%.1f" y="%.1f" font-family="Arial, Microsoft YaHei, sans-serif" font-size="13" fill="%s">%s</text>`, x+22, rowY, style.MutedText, svgEsc(item.Name)),
		)
	}
	return elements
}

func renderBarChart(title string, data map[string]interface{}, options map[string]interface{}) (string, chartRenderMeta, error) {
	style := styleFromOptions(options)
	categories, _, err := firstStringSliceField(data, "categories")
	if err != nil {
		return "", chartRenderMeta{}, err
	}
	if len(categories) == 0 {
		return "", chartRenderMeta{}, fmt.Errorf("data.categories must contain at least 1 item")
	}
	series, err := seriesField(data, "series", len(categories), style, 0, false)
	if err != nil {
		return "", chartRenderMeta{}, err
	}
	maxValue, err := chartMaxValue(data, series)
	if err != nil {
		return "", chartRenderMeta{}, err
	}
	if err := validateSeriesMax(series, maxValue); err != nil {
		return "", chartRenderMeta{}, err
	}

	width := intOption(options, "width", 900, 480, 1600)
	height := intOption(options, "height", 620, 420, 1200)
	showValues := boolOption(options, "show_values", true)
	showLegend := boolOption(options, "legend", true)
	showGrid := boolOption(options, "grid", true)
	if strings.TrimSpace(title) == "" {
		title = "Bar Chart"
	}

	frame := newChartFrame(width, height)
	elements := renderChartHeader(title, width, style)
	elements = append(elements, renderCartesianGrid(frame, maxValue, style, showGrid)...)
	elements = append(elements, renderCategoryLabels(frame, categories, style)...)

	groupWidth := frame.plotWidth / float64(len(categories))
	barBand := groupWidth * 0.72
	barWidth := math.Max(4, barBand/float64(len(series)))
	for categoryIndex := range categories {
		groupLeft := frame.left + groupWidth*float64(categoryIndex) + (groupWidth-barBand)/2
		for seriesIndex, item := range series {
			value := item.Values[categoryIndex]
			barHeight := frame.plotHeight * value / maxValue
			x := groupLeft + barWidth*float64(seriesIndex)
			y := frame.bottom - barHeight
			elements = append(elements, fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="3" fill="%s"/>`, x, y, barWidth*0.86, barHeight, item.Color))
			if showValues {
				elements = append(elements, fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" font-family="Arial, sans-serif" font-size="11" fill="%s">%s</text>`, x+barWidth*0.43, y-6, style.Text, svgEsc(formatFloat(value))))
			}
		}
	}
	if showLegend {
		elements = append(elements, renderLegend(float64(width)-210, 72, series, style)...)
	}

	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img">%s</svg>`,
		width, height, width, height, strings.Join(elements, "\n"))
	return svg, chartRenderMeta{ChartType: "bar", XCount: len(categories), SeriesCount: len(series)}, nil
}

func renderLineChart(title string, data map[string]interface{}, options map[string]interface{}) (string, chartRenderMeta, error) {
	style := styleFromOptions(options)
	xAxis, _, err := firstStringSliceField(data, "x_axis", "categories")
	if err != nil {
		return "", chartRenderMeta{}, err
	}
	if len(xAxis) == 0 {
		return "", chartRenderMeta{}, fmt.Errorf("data.x_axis must contain at least 1 item")
	}
	series, err := seriesField(data, "series", len(xAxis), style, 0, false)
	if err != nil {
		return "", chartRenderMeta{}, err
	}
	maxValue, err := chartMaxValue(data, series)
	if err != nil {
		return "", chartRenderMeta{}, err
	}
	if err := validateSeriesMax(series, maxValue); err != nil {
		return "", chartRenderMeta{}, err
	}

	width := intOption(options, "width", 900, 480, 1600)
	height := intOption(options, "height", 620, 420, 1200)
	showValues := boolOption(options, "show_values", true)
	showLegend := boolOption(options, "legend", true)
	showGrid := boolOption(options, "grid", true)
	if strings.TrimSpace(title) == "" {
		title = "Line Chart"
	}

	frame := newChartFrame(width, height)
	elements := renderChartHeader(title, width, style)
	elements = append(elements, renderCartesianGrid(frame, maxValue, style, showGrid)...)
	elements = append(elements, renderCategoryLabels(frame, xAxis, style)...)

	xAt := func(index int) float64 {
		if len(xAxis) == 1 {
			return frame.left + frame.plotWidth/2
		}
		return frame.left + frame.plotWidth*float64(index)/float64(len(xAxis)-1)
	}
	yAt := func(value float64) float64 {
		return frame.bottom - frame.plotHeight*value/maxValue
	}
	for _, item := range series {
		points := make([]string, 0, len(item.Values))
		for index, value := range item.Values {
			points = append(points, fmt.Sprintf("%.1f,%.1f", xAt(index), yAt(value)))
		}
		elements = append(elements, fmt.Sprintf(`<polyline points="%s" fill="none" stroke="%s" stroke-width="3" stroke-linejoin="round" stroke-linecap="round"/>`, strings.Join(points, " "), item.Color))
		for index, value := range item.Values {
			x := xAt(index)
			y := yAt(value)
			elements = append(elements, fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="4" fill="%s" stroke="#ffffff" stroke-width="1.5"/>`, x, y, item.Color))
			if showValues {
				elements = append(elements, fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" font-family="Arial, sans-serif" font-size="11" fill="%s">%s</text>`, x, y-10, style.Text, svgEsc(formatFloat(value))))
			}
		}
	}
	if showLegend {
		elements = append(elements, renderLegend(float64(width)-210, 72, series, style)...)
	}

	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img">%s</svg>`,
		width, height, width, height, strings.Join(elements, "\n"))
	return svg, chartRenderMeta{ChartType: "line", XCount: len(xAxis), SeriesCount: len(series)}, nil
}

func validateSeriesMax(series []chartSeries, maxValue float64) error {
	for seriesIndex, item := range series {
		for valueIndex, value := range item.Values {
			if value > maxValue {
				return fmt.Errorf("data.series[%d].values[%d] must be between 0 and %s", seriesIndex, valueIndex, formatFloat(maxValue))
			}
		}
	}
	return nil
}
