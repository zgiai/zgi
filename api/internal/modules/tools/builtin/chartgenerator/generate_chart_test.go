package chartgenerator

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"

	"github.com/stretchr/testify/require"
)

func TestRenderRadarChartProducesSVG(t *testing.T) {
	svg, meta, err := renderRadarChart(
		"Score Comparison",
		map[string]interface{}{
			"dimensions": []interface{}{"Chinese", "Math", "English", "Physics", "Chemistry", "Biology", "History"},
			"max_value":  100,
			"series": []interface{}{
				map[string]interface{}{"name": "Class Average", "values": []interface{}{78, 82, 80, 75, 73, 76, 79}},
				map[string]interface{}{"name": "Student", "values": []interface{}{88, 92, 84, 81, 77, 86, 83}},
			},
		},
		map[string]interface{}{"show_values": true},
	)
	require.NoError(t, err)
	require.Equal(t, chartRenderMeta{ChartType: "radar", XCount: 7, SeriesCount: 2}, meta)
	require.True(t, strings.HasPrefix(svg, `<svg `))
	require.Contains(t, svg, "Score Comparison")
	require.Contains(t, svg, "History")
	require.Contains(t, svg, "Class Average")
	require.Contains(t, svg, "Student")
}

func TestRenderRadarChartValidatesSeriesLength(t *testing.T) {
	_, _, err := renderRadarChart(
		"",
		map[string]interface{}{
			"dimensions": []interface{}{"Chinese", "Math", "English"},
			"series": []interface{}{
				map[string]interface{}{"name": "Student", "values": []interface{}{88, 92}},
			},
		},
		nil,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must contain 3 numbers")
}

func TestRenderBarChartProducesSVG(t *testing.T) {
	svg, meta, err := renderBarChart(
		"Subject Scores",
		map[string]interface{}{
			"categories": []interface{}{"Chinese", "Math", "English"},
			"max_value":  100,
			"series": []interface{}{
				map[string]interface{}{"name": "Student", "values": []interface{}{88, 92, 85}},
				map[string]interface{}{"name": "Class Average", "values": []interface{}{78, 80, 79}},
			},
		},
		map[string]interface{}{"style": "comparison", "show_values": true},
	)
	require.NoError(t, err)
	require.Equal(t, chartRenderMeta{ChartType: "bar", XCount: 3, SeriesCount: 2}, meta)
	require.Contains(t, svg, "Subject Scores")
	require.Contains(t, svg, "<rect")
	require.Contains(t, svg, "Student")
	require.Contains(t, svg, "Class Average")
}

func TestRenderLineChartProducesSVG(t *testing.T) {
	svg, meta, err := renderLineChart(
		"Score Trend",
		map[string]interface{}{
			"x_axis":    []interface{}{"First", "Second", "Third"},
			"max_value": 100,
			"series": []interface{}{
				map[string]interface{}{"name": "Math", "values": []interface{}{78, 85, 92}},
				map[string]interface{}{"name": "English", "values": []interface{}{82, 84, 88}},
			},
		},
		map[string]interface{}{"style": "teaching", "show_values": true},
	)
	require.NoError(t, err)
	require.Equal(t, chartRenderMeta{ChartType: "line", XCount: 3, SeriesCount: 2}, meta)
	require.Contains(t, svg, "Score Trend")
	require.Contains(t, svg, "<polyline")
	require.Contains(t, svg, "Math")
	require.Contains(t, svg, "Third")
}

func TestRenderCartesianChartValidatesSeriesLength(t *testing.T) {
	_, _, err := renderBarChart(
		"",
		map[string]interface{}{
			"categories": []interface{}{"Chinese", "Math", "English"},
			"series": []interface{}{
				map[string]interface{}{"name": "Student", "values": []interface{}{88, 92}},
			},
		},
		nil,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must contain 3 numbers")
}

func TestRenderChartsAcceptStringLabels(t *testing.T) {
	_, meta, err := renderBarChart(
		"CSV Categories",
		map[string]interface{}{
			"categories": "Chinese,Math,English",
			"series": []interface{}{
				map[string]interface{}{"name": "Student", "values": []interface{}{88, 92, 85}},
			},
		},
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, chartRenderMeta{ChartType: "bar", XCount: 3, SeriesCount: 1}, meta)

	_, meta, err = renderLineChart(
		"JSON X Axis",
		map[string]interface{}{
			"x_axis": `["First","Second","Third"]`,
			"series": []interface{}{
				map[string]interface{}{"name": "Math", "values": []interface{}{78, 85, 92}},
			},
		},
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, chartRenderMeta{ChartType: "line", XCount: 3, SeriesCount: 1}, meta)
}

func TestRenderPieChartProducesSVG(t *testing.T) {
	svg, meta, err := renderPieChart(
		"Score Band Share",
		map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"label": "90-100", "value": 2},
				map[string]interface{}{"label": "80-89", "value": 3},
				map[string]interface{}{"label": "70-79", "value": 2},
			},
		},
		map[string]interface{}{"show_values": true},
	)
	require.NoError(t, err)
	require.Equal(t, chartRenderMeta{ChartType: "pie", XCount: 3, SeriesCount: 1}, meta)
	require.Contains(t, svg, "Score Band Share")
	require.Contains(t, svg, "<path")
	require.Contains(t, svg, "90-100")
}

func TestRenderDoughnutChartProducesSVG(t *testing.T) {
	svg, meta, err := renderDoughnutChart(
		"Score Band Share",
		map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"label": "90-100", "value": 2},
				map[string]interface{}{"label": "80-89", "value": 3},
			},
		},
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, chartRenderMeta{ChartType: "doughnut", XCount: 2, SeriesCount: 1}, meta)
	require.Contains(t, svg, "Score Band Share")
	require.Contains(t, svg, "<circle")
	require.Contains(t, svg, "80-89")
}

func TestRenderScatterChartProducesSVG(t *testing.T) {
	svg, meta, err := renderScatterChart(
		"Rank Score",
		map[string]interface{}{
			"x_label": "Rank",
			"y_label": "Score",
			"x_min":   1,
			"x_max":   8,
			"y_min":   0,
			"y_max":   100,
			"points": []interface{}{
				map[string]interface{}{"x": 1, "y": 98, "label": "Zhang San"},
				map[string]interface{}{"x": 2, "y": 98, "label": "Sun Ba"},
				map[string]interface{}{"x": 3, "y": 88, "label": "Li Si"},
			},
		},
		map[string]interface{}{"show_labels": true},
	)
	require.NoError(t, err)
	require.Equal(t, chartRenderMeta{ChartType: "scatter", XCount: 3, SeriesCount: 1}, meta)
	require.Contains(t, svg, "Rank Score")
	require.Contains(t, svg, "<circle")
	require.Contains(t, svg, "Zhang San")
}

func TestRenderScatterChartAcceptsSinglePoint(t *testing.T) {
	svg, meta, err := renderScatterChart(
		"Single Point",
		map[string]interface{}{
			"points": []interface{}{
				map[string]interface{}{"x": 1, "y": 98, "label": "Zhang San"},
			},
		},
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, chartRenderMeta{ChartType: "scatter", XCount: 1, SeriesCount: 1}, meta)
	require.Contains(t, svg, "Zhang San")
}

func TestRenderScatterChartAcceptsSameYValues(t *testing.T) {
	svg, meta, err := renderScatterChart(
		"Same Score",
		map[string]interface{}{
			"points": []interface{}{
				map[string]interface{}{"x": 1, "y": 98, "label": "Zhang San"},
				map[string]interface{}{"x": 2, "y": 98, "label": "Sun Ba"},
			},
		},
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, chartRenderMeta{ChartType: "scatter", XCount: 2, SeriesCount: 1}, meta)
	require.Contains(t, svg, "Same Score")
}

func TestRenderScatterChartRejectsExplicitInvalidBounds(t *testing.T) {
	_, _, err := renderScatterChart(
		"Invalid Bounds",
		map[string]interface{}{
			"x_min": 1,
			"x_max": 1,
			"points": []interface{}{
				map[string]interface{}{"x": 1, "y": 98},
			},
		},
		nil,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "data.x_max must be greater than data.x_min")
}

func TestRenderScoreDistributionChartCountsRawScores(t *testing.T) {
	svg, meta, err := renderScoreDistributionChart(
		"Score Distribution",
		map[string]interface{}{
			"scores": []interface{}{
				map[string]interface{}{"label": "Zhang San", "value": 98},
				map[string]interface{}{"label": "Sun Ba", "value": 98},
				map[string]interface{}{"label": "Li Si", "value": 88},
				map[string]interface{}{"label": "Wu Shi", "value": 82},
				map[string]interface{}{"label": "Zhao Liu", "value": 67},
			},
			"bands": []interface{}{
				map[string]interface{}{"label": "90-100", "min": 90, "max": 100},
				map[string]interface{}{"label": "80-89", "min": 80, "max": 89},
				map[string]interface{}{"label": "60-69", "min": 60, "max": 69},
			},
		},
		map[string]interface{}{"show_values": true},
	)
	require.NoError(t, err)
	require.Equal(t, chartRenderMeta{ChartType: "score_distribution", XCount: 3, SeriesCount: 1}, meta)
	require.Contains(t, svg, "Score Distribution")
	require.Contains(t, svg, "90-100")
	require.Contains(t, svg, ">2<")
}

func TestRenderScoreDistributionChartAcceptsPrecomputedCounts(t *testing.T) {
	_, meta, err := renderScoreDistributionChart(
		"",
		map[string]interface{}{
			"bands": []interface{}{
				map[string]interface{}{"label": "90-100", "count": 2},
				map[string]interface{}{"label": "80-89", "count": 3},
			},
		},
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, chartRenderMeta{ChartType: "score_distribution", XCount: 2, SeriesCount: 1}, meta)
}

func TestRenderChartRejectsUnsupportedType(t *testing.T) {
	_, _, err := renderChart("unknown", "", map[string]interface{}{}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported chart_type")
}

func TestBuildChartFilenameAddsSVGExtension(t *testing.T) {
	require.Equal(t, "score-radar.svg", buildChartFilename("score-radar", ".svg"))
	require.Equal(t, "score-radar.svg", buildChartFilename("score-radar.pdf", ".svg"))
	require.Equal(t, "chart.svg", buildChartFilename("../", ".svg"))
}

func TestResolveChartFileLifecycleDefaultsToTemporary(t *testing.T) {
	lifecycle, err := resolveChartFileLifecycle("")
	require.NoError(t, err)
	require.Equal(t, tool_file.ToolFileLifecycleTemporary, lifecycle)
}
