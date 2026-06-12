package architecturediagram

import (
	"fmt"
	"html"
	"sort"
	"strings"
)

type point struct {
	X float64
	Y float64
}

type diagramPalette struct {
	Background string
	Text       string
	Muted      string
	Line       string
	Fill       string
	Accent     string
	Accent2    string
	Header     string
	Canvas     string
	Border     string
}

func paletteForStyle(style string) diagramPalette {
	switch style {
	case "paper":
		return diagramPalette{Background: "#edebe1", Text: "#1a1a18", Muted: "#6b6961", Line: "#d4d1c7", Fill: "#f5f3eb", Accent: "#3d5af1", Accent2: "#2d8a56", Header: "#f5f3eb", Canvas: "#edebe1", Border: "#d4d1c7"}
	case "business":
		return diagramPalette{Background: "#ffffff", Text: "#0f172a", Muted: "#64748b", Line: "#cbd5e1", Fill: "#f8fafc", Accent: "#0f766e", Accent2: "#2563eb", Header: "#e0f2fe", Canvas: "#f8fafc", Border: "#cbd5e1"}
	case "presentation":
		return diagramPalette{Background: "#ffffff", Text: "#111827", Muted: "#6b7280", Line: "#d1d5db", Fill: "#f9fafb", Accent: "#7c3aed", Accent2: "#2563eb", Header: "#ede9fe", Canvas: "#f9fafb", Border: "#d1d5db"}
	case "technical":
		return diagramPalette{Background: "#ffffff", Text: "#111827", Muted: "#4b5563", Line: "#9ca3af", Fill: "#f3f4f6", Accent: "#2563eb", Accent2: "#16a34a", Header: "#dbeafe", Canvas: "#ffffff", Border: "#d1d5db"}
	default:
		return diagramPalette{Background: "#ffffff", Text: "#111827", Muted: "#6b7280", Line: "#d1d5db", Fill: "#ffffff", Accent: "#2563eb", Accent2: "#64748b", Header: "#eff6ff", Canvas: "#ffffff", Border: "#d1d5db"}
	}
}

func renderDiagram(spec diagramSpec) (string, string, diagramRenderMeta, error) {
	var svg string
	var meta diagramRenderMeta
	switch spec.DiagramType {
	case "system_architecture", "agent_architecture", "data_flow", "flowchart":
		svg = renderNodeEdgeDiagram(spec)
		meta = diagramRenderMeta{DiagramType: spec.DiagramType, NodeCount: len(spec.Nodes), EdgeCount: len(spec.Edges)}
	case "comparison_matrix":
		svg = renderComparisonMatrix(spec)
		meta = diagramRenderMeta{DiagramType: spec.DiagramType, NodeCount: len(spec.Rows), EdgeCount: len(spec.Columns)}
	case "sequence":
		svg = renderSequenceDiagram(spec)
		meta = diagramRenderMeta{DiagramType: spec.DiagramType, NodeCount: len(spec.Participants), EdgeCount: len(spec.Messages)}
	case "state":
		nodeSpec := spec
		nodeSpec.Nodes = spec.States
		nodeSpec.Edges = spec.Transitions
		svg = renderNodeEdgeDiagram(nodeSpec)
		meta = diagramRenderMeta{DiagramType: spec.DiagramType, NodeCount: len(spec.States), EdgeCount: len(spec.Transitions)}
	case "er":
		svg = renderERDiagram(spec)
		meta = diagramRenderMeta{DiagramType: spec.DiagramType, NodeCount: len(spec.Entities), EdgeCount: len(spec.Relations)}
	default:
		return "", "", diagramRenderMeta{}, fmt.Errorf("unsupported diagram_type: %s", spec.DiagramType)
	}
	return svg, renderHTMLDocument(spec, svg), meta, nil
}

func renderNodeEdgeDiagram(spec diagramSpec) string {
	p := paletteForStyle(spec.Options.Style)
	titleY := 44
	top := 122
	boxW := 220
	boxH := 88
	nodeGapX := 88
	nodeGapY := 64
	marginX := 72
	marginBottom := 64
	positions := map[string]point{}

	layers := sortedLayers(spec.Nodes)
	layerIndex := map[string]int{}
	for index, layer := range layers {
		layerIndex[layer] = index
	}
	layerBuckets := make([][]diagramNode, len(layers))
	for _, node := range spec.Nodes {
		layer := firstNonEmpty(node.Layer, node.Group, "default")
		layerBuckets[layerIndex[layer]] = append(layerBuckets[layerIndex[layer]], node)
	}
	sortLayerBucketsByGroup(layerBuckets, spec)
	maxBucketSize := 1
	for _, bucket := range layerBuckets {
		maxBucketSize = maxInt(maxBucketSize, len(bucket))
	}
	width := spec.Options.Width
	height := spec.Options.Height
	if spec.Options.Direction == "top_to_bottom" {
		width = maxInt(width, marginX*2+maxBucketSize*boxW+maxInt(0, maxBucketSize-1)*nodeGapX)
		height = maxInt(height, top+marginBottom+len(layers)*boxH+maxInt(0, len(layers)-1)*nodeGapY)
	} else {
		width = maxInt(width, marginX*2+len(layers)*boxW+maxInt(0, len(layers)-1)*nodeGapX)
		height = maxInt(height, top+marginBottom+maxBucketSize*boxH+maxInt(0, maxBucketSize-1)*nodeGapY)
	}
	spec.Options.Width = width
	spec.Options.Height = height

	left := marginX
	for layerIdx, bucket := range layerBuckets {
		for itemIdx, node := range bucket {
			if spec.Options.Direction == "top_to_bottom" {
				x := float64(width) / 2
				if len(bucket) > 1 {
					rowWidth := float64(len(bucket)*boxW + (len(bucket)-1)*nodeGapX)
					x = (float64(width)-rowWidth)/2 + float64(boxW)/2 + float64(itemIdx*(boxW+nodeGapX))
				}
				y := float64(top) + float64(boxH)/2 + float64(layerIdx*(boxH+nodeGapY))
				positions[node.ID] = point{X: x, Y: y}
			} else {
				x := float64(left) + float64(boxW)/2 + float64(layerIdx*(boxW+nodeGapX))
				y := float64(top) + float64(boxH)/2
				if len(bucket) > 1 {
					columnHeight := float64(len(bucket)*boxH + (len(bucket)-1)*nodeGapY)
					y = (float64(height)-columnHeight)/2 + float64(boxH)/2 + float64(itemIdx*(boxH+nodeGapY))
				}
				positions[node.ID] = point{X: x, Y: y}
			}
		}
	}

	var b strings.Builder
	writeSVGStart(&b, width, height, p, spec.Title)
	writeGroupBoxes(&b, spec, positions, boxW, boxH)
	incoming := incomingEdgeCounts(spec.Edges)
	for _, edge := range spec.Edges {
		from := positions[edge.From]
		to := positions[edge.To]
		route := nodeEdgeRoute(from, to, boxW, boxH, spec.Options.Direction)
		fmt.Fprintf(&b, `<path d="%s" fill="none" stroke="%s" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>`, route.Path, p.Line)
		if incoming[edge.To] > 1 {
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="4.5" fill="%s"/>`, route.End.X, route.End.Y, p.Line)
		} else {
			fmt.Fprintf(&b, `<polygon points="%s" fill="%s"/>`, arrowPoint(route.ArrowFrom.X, route.ArrowFrom.Y, route.End.X, route.End.Y), p.Line)
		}
		if spec.Options.ShowLabels && edge.Label != "" {
			label := compactText(edge.Label, 18)
			labelW := maxInt(48, textApproxWidth(label, 12)+14)
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%d" height="20" rx="5" fill="#ffffff" opacity="0.92"/>`, route.Label.X-float64(labelW)/2, route.Label.Y-15, labelW)
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" font-size="12" fill="%s">%s</text>`, route.Label.X, route.Label.Y, p.Muted, svgEsc(label))
		}
	}
	for _, node := range spec.Nodes {
		pos := positions[node.ID]
		x := pos.X - float64(boxW)/2
		y := pos.Y - float64(boxH)/2
		accent := colorForNode(node)
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%d" height="%d" rx="10" fill="%s" stroke="%s" stroke-width="1.2"/>`, x, y, boxW, boxH, p.Fill, p.Border)
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%d" height="4" rx="2" fill="%s"/>`, x, y, boxW, accent)
		writeWrappedCenteredText(&b, pos.X, pos.Y-10, node.Label, boxW-24, 15, 2, "700", p.Text)
		if node.Type != "" {
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" font-size="12" fill="%s">%s</text>`, pos.X, pos.Y+30, p.Muted, svgEsc(compactText(node.Type, 22)))
		}
	}
	writeLegend(&b, spec, width, height)
	writeSVGEnd(&b, spec, p, titleY)
	return b.String()
}

type groupBox struct {
	ID    string
	Label string
	MinX  float64
	MinY  float64
	MaxX  float64
	MaxY  float64
	Color string
	Fill  string
	Seen  bool
	Index int
}

func writeGroupBoxes(b *strings.Builder, spec diagramSpec, positions map[string]point, boxW int, boxH int) {
	groups := groupBoxesForNodes(spec, positions, boxW, boxH)
	for _, group := range groups {
		fmt.Fprintf(b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="14" fill="%s" stroke="%s" stroke-width="1.4" stroke-dasharray="7 5" opacity="0.96"/>`, group.MinX, group.MinY, group.MaxX-group.MinX, group.MaxY-group.MinY, group.Fill, group.Color)
		fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="13" font-weight="700" fill="%s">%s</text>`, group.MinX+14, group.MinY+20, group.Color, svgEsc(group.Label))
	}
}

func groupBoxesForNodes(spec diagramSpec, positions map[string]point, boxW int, boxH int) []groupBox {
	labelByID := map[string]string{}
	for _, group := range spec.Groups {
		labelByID[group.ID] = firstNonEmpty(group.Label, group.ID)
	}
	groupOrder := []string{}
	boxes := map[string]*groupBox{}
	for _, node := range spec.Nodes {
		groupID := renderGroupID(node)
		if groupID == "" || groupID == "default" {
			continue
		}
		pos, ok := positions[node.ID]
		if !ok {
			continue
		}
		box, ok := boxes[groupID]
		if !ok {
			index := len(groupOrder)
			color, fill := groupColor(index)
			box = &groupBox{
				ID:    groupID,
				Label: firstNonEmpty(labelByID[groupID], groupID),
				Color: color,
				Fill:  fill,
				Index: index,
			}
			boxes[groupID] = box
			groupOrder = append(groupOrder, groupID)
		}
		x1 := pos.X - float64(boxW)/2 - groupPaddingX()
		y1 := pos.Y - float64(boxH)/2 - groupPaddingTop()
		x2 := pos.X + float64(boxW)/2 + groupPaddingX()
		y2 := pos.Y + float64(boxH)/2 + groupPaddingBottom()
		if !box.Seen {
			box.MinX, box.MinY, box.MaxX, box.MaxY = x1, y1, x2, y2
			box.Seen = true
			continue
		}
		if x1 < box.MinX {
			box.MinX = x1
		}
		if y1 < box.MinY {
			box.MinY = y1
		}
		if x2 > box.MaxX {
			box.MaxX = x2
		}
		if y2 > box.MaxY {
			box.MaxY = y2
		}
	}
	result := make([]groupBox, 0, len(groupOrder))
	for _, id := range groupOrder {
		if box := boxes[id]; box != nil && box.Seen {
			result = append(result, *box)
		}
	}
	return result
}

func sortLayerBucketsByGroup(layerBuckets [][]diagramNode, spec diagramSpec) {
	rank := groupRank(spec)
	for idx := range layerBuckets {
		sort.SliceStable(layerBuckets[idx], func(i, j int) bool {
			left := renderGroupID(layerBuckets[idx][i])
			right := renderGroupID(layerBuckets[idx][j])
			leftRank := groupRankValue(rank, left)
			rightRank := groupRankValue(rank, right)
			if leftRank != rightRank {
				return leftRank < rightRank
			}
			return layerBuckets[idx][i].ID < layerBuckets[idx][j].ID
		})
	}
}

func groupRank(spec diagramSpec) map[string]int {
	rank := map[string]int{}
	for _, group := range spec.Groups {
		id := strings.TrimSpace(group.ID)
		if id == "" {
			continue
		}
		if _, ok := rank[id]; !ok {
			rank[id] = len(rank)
		}
	}
	for _, node := range spec.Nodes {
		id := renderGroupID(node)
		if id == "" {
			continue
		}
		if _, ok := rank[id]; !ok {
			rank[id] = len(rank)
		}
	}
	return rank
}

func groupRankValue(rank map[string]int, id string) int {
	if value, ok := rank[id]; ok {
		return value
	}
	return len(rank) + 1
}

func renderGroupID(node diagramNode) string {
	if strings.TrimSpace(node.Group) != "" {
		return strings.TrimSpace(node.Group)
	}
	layer := strings.TrimSpace(node.Layer)
	if layer == "" || isNumericLayer(layer) {
		return ""
	}
	return layer
}

func isNumericLayer(value string) bool {
	for _, r := range strings.TrimSpace(value) {
		if r < '0' || r > '9' {
			return false
		}
	}
	return strings.TrimSpace(value) != ""
}

func groupPaddingX() float64 {
	return 18
}

func groupPaddingTop() float64 {
	return 26
}

func groupPaddingBottom() float64 {
	return 16
}

func groupColor(index int) (string, string) {
	palette := []struct {
		stroke string
		fill   string
	}{
		{"#2563eb", "#eff6ff"},
		{"#0f766e", "#ecfdf5"},
		{"#9333ea", "#faf5ff"},
		{"#ea580c", "#fff7ed"},
		{"#475569", "#f8fafc"},
		{"#16a34a", "#f0fdf4"},
		{"#dc2626", "#fef2f2"},
		{"#0891b2", "#ecfeff"},
	}
	item := palette[index%len(palette)]
	return item.stroke, item.fill
}

func colorForNode(node diagramNode) string {
	value := strings.ToLower(firstNonEmpty(node.Group, node.Type, node.Layer))
	switch {
	case strings.Contains(value, "front"), strings.Contains(value, "web"), strings.Contains(value, "client"), strings.Contains(value, "input"):
		return "#3d5af1"
	case strings.Contains(value, "back"), strings.Contains(value, "api"), strings.Contains(value, "service"), strings.Contains(value, "agent"):
		return "#2d8a56"
	case strings.Contains(value, "data"), strings.Contains(value, "db"), strings.Contains(value, "database"), strings.Contains(value, "memory"):
		return "#c47d1a"
	case strings.Contains(value, "tool"), strings.Contains(value, "workflow"):
		return "#7c4dff"
	case strings.Contains(value, "model"), strings.Contains(value, "llm"):
		return "#4f57f1"
	case strings.Contains(value, "output"), strings.Contains(value, "notify"):
		return "#1a8a7a"
	case strings.Contains(value, "external"):
		return "#d44332"
	default:
		return "#3d5af1"
	}
}

func writeLegend(b *strings.Builder, spec diagramSpec, width int, height int) {
	groups := []diagramGroup{}
	if len(spec.Groups) > 0 {
		groups = append(groups, spec.Groups...)
	} else {
		seen := map[string]struct{}{}
		for _, node := range spec.Nodes {
			id := renderGroupID(node)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			groups = append(groups, diagramGroup{ID: id, Label: id})
		}
	}
	if len(groups) == 0 {
		return
	}
	x := 72.0
	y := float64(height - 26)
	for index, group := range groups {
		if index >= 8 {
			break
		}
		label := compactText(firstNonEmpty(group.Label, group.ID), 16)
		color, _ := groupColor(index)
		fmt.Fprintf(b, `<rect x="%.1f" y="%.1f" width="9" height="9" rx="2" fill="%s"/>`, x, y-8, color)
		fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="12" fill="#6b6961">%s</text>`, x+15, y, svgEsc(label))
		x += float64(24 + textApproxWidth(label, 12))
	}
}

func incomingEdgeCounts(edges []diagramEdge) map[string]int {
	counts := map[string]int{}
	for _, edge := range edges {
		counts[edge.To]++
	}
	return counts
}

func renderComparisonMatrix(spec diagramSpec) string {
	p := paletteForStyle(spec.Options.Style)
	width := spec.Options.Width
	height := spec.Options.Height
	left := 70
	top := 92
	rowH := 54
	firstColW := 190
	colW := maxInt(120, (width-left-70-firstColW)/maxInt(1, len(spec.Columns)))
	var b strings.Builder
	writeSVGStart(&b, width, height, p, spec.Title)
	fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s" stroke="%s"/>`, left, top, firstColW+colW*len(spec.Columns), rowH, p.Header, p.Line)
	for idx, column := range spec.Columns {
		x := left + firstColW + idx*colW
		fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="middle" font-size="13" font-weight="700" fill="%s">%s</text>`, x+colW/2, top+33, p.Text, svgEsc(column))
	}
	for rowIdx, row := range spec.Rows {
		y := top + rowH*(rowIdx+1)
		fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s" stroke="%s"/>`, left, y, firstColW, rowH, p.Fill, p.Line)
		fmt.Fprintf(&b, `<text x="%d" y="%d" font-size="13" font-weight="700" fill="%s">%s</text>`, left+14, y+33, p.Text, svgEsc(row))
		for colIdx := range spec.Columns {
			x := left + firstColW + colIdx*colW
			fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" fill="#ffffff" stroke="%s"/>`, x, y, colW, rowH, p.Line)
			fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="middle" font-size="13" fill="%s">%s</text>`, x+colW/2, y+33, p.Text, svgEsc(spec.Cells[rowIdx][colIdx]))
		}
	}
	writeSVGEnd(&b, spec, p, 44)
	return b.String()
}

func renderSequenceDiagram(spec diagramSpec) string {
	p := paletteForStyle(spec.Options.Style)
	width := spec.Options.Width
	height := maxInt(spec.Options.Height, 160+len(spec.Messages)*64)
	left := 80
	top := 110
	bottom := height - 70
	step := float64(width-left-80) / float64(maxInt(1, len(spec.Participants)-1))
	xByName := map[string]float64{}
	var b strings.Builder
	writeSVGStart(&b, width, height, p, spec.Title)
	for idx, participant := range spec.Participants {
		x := float64(left)
		if len(spec.Participants) > 1 {
			x += step * float64(idx)
		}
		xByName[participant] = x
		fmt.Fprintf(&b, `<rect x="%.1f" y="82" width="150" height="42" rx="8" transform="translate(-75 0)" fill="%s" stroke="%s"/>`, x, p.Fill, p.Accent)
		fmt.Fprintf(&b, `<text x="%.1f" y="108" text-anchor="middle" font-size="13" font-weight="700" fill="%s">%s</text>`, x, p.Text, svgEsc(participant))
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%d" x2="%.1f" y2="%d" stroke="%s" stroke-dasharray="6 6"/>`, x, top, x, bottom, p.Line)
	}
	for idx, message := range spec.Messages {
		y := float64(top + 38 + idx*58)
		x1 := xByName[message.From]
		x2 := xByName[message.To]
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="2"/>`, x1, y, x2, y, p.Accent)
		fmt.Fprintf(&b, `<polygon points="%s" fill="%s"/>`, arrowPoint(x1, y, x2, y), p.Accent)
		if message.Label != "" {
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" font-size="12" fill="%s">%s</text>`, (x1+x2)/2, y-8, p.Text, svgEsc(message.Label))
		}
	}
	writeSVGEnd(&b, spec, p, 44)
	return b.String()
}

func renderERDiagram(spec diagramSpec) string {
	p := paletteForStyle(spec.Options.Style)
	width := spec.Options.Width
	height := spec.Options.Height
	positions := map[string]point{}
	cols := maxInt(1, minInt(3, len(spec.Entities)))
	rows := (len(spec.Entities) + cols - 1) / cols
	cardW := 230
	cardH := 120
	for idx, entity := range spec.Entities {
		col := idx % cols
		row := idx / cols
		positions[entity.ID] = point{
			X: float64(120 + col*((width-240)/maxInt(1, cols-1))),
			Y: float64(130 + row*((height-230)/maxInt(1, rows-1))),
		}
	}
	var b strings.Builder
	writeSVGStart(&b, width, height, p, spec.Title)
	for _, relation := range spec.Relations {
		from := positions[relation.From]
		to := positions[relation.To]
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="2"/>`, from.X, from.Y, to.X, to.Y, p.Line)
		if relation.Label != "" {
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" font-size="12" fill="%s">%s</text>`, (from.X+to.X)/2, (from.Y+to.Y)/2-8, p.Muted, svgEsc(relation.Label))
		}
	}
	for _, entity := range spec.Entities {
		pos := positions[entity.ID]
		x := pos.X - float64(cardW)/2
		y := pos.Y - float64(cardH)/2
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%d" height="%d" rx="8" fill="#ffffff" stroke="%s"/>`, x, y, cardW, cardH, p.Accent)
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%d" height="34" rx="8" fill="%s"/>`, x, y, cardW, p.Header)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" font-size="14" font-weight="700" fill="%s">%s</text>`, pos.X, y+22, p.Text, svgEsc(entity.Label))
		for idx, field := range entity.Fields {
			if idx > 4 {
				break
			}
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-size="12" fill="%s">%s</text>`, x+14, y+56+float64(idx*17), p.Muted, svgEsc(field))
		}
	}
	writeSVGEnd(&b, spec, p, 44)
	return b.String()
}

type edgeRoute struct {
	Path      string
	End       point
	ArrowFrom point
	Label     point
}

func nodeEdgeRoute(from point, to point, boxW int, boxH int, direction string) edgeRoute {
	if direction == "top_to_bottom" {
		start := point{X: from.X, Y: from.Y + float64(boxH)/2}
		end := point{X: to.X, Y: to.Y - float64(boxH)/2}
		if to.Y < from.Y {
			start = point{X: from.X, Y: from.Y - float64(boxH)/2}
			end = point{X: to.X, Y: to.Y + float64(boxH)/2}
		}
		midY := (start.Y + end.Y) / 2
		return edgeRoute{
			Path:      fmt.Sprintf("M %.1f %.1f V %.1f H %.1f V %.1f", start.X, start.Y, midY, end.X, end.Y),
			End:       end,
			ArrowFrom: point{X: end.X, Y: midY},
			Label:     point{X: (start.X + end.X) / 2, Y: midY - 6},
		}
	}
	start := point{X: from.X + float64(boxW)/2, Y: from.Y}
	end := point{X: to.X - float64(boxW)/2, Y: to.Y}
	if to.X < from.X {
		start = point{X: from.X - float64(boxW)/2, Y: from.Y}
		end = point{X: to.X + float64(boxW)/2, Y: to.Y}
	}
	midX := (start.X + end.X) / 2
	return edgeRoute{
		Path:      fmt.Sprintf("M %.1f %.1f H %.1f V %.1f H %.1f", start.X, start.Y, midX, end.Y, end.X),
		End:       end,
		ArrowFrom: point{X: midX, Y: end.Y},
		Label:     point{X: midX, Y: (start.Y+end.Y)/2 - 6},
	}
}

func writeWrappedCenteredText(b *strings.Builder, x float64, centerY float64, text string, maxWidth int, fontSize int, maxLines int, weight string, fill string) {
	lines := wrapTextLines(text, maxWidth, fontSize, maxLines)
	if len(lines) == 0 {
		return
	}
	lineHeight := float64(fontSize + 4)
	startY := centerY - lineHeight*float64(len(lines)-1)/2
	for index, line := range lines {
		fmt.Fprintf(b, `<text x="%.1f" y="%.1f" text-anchor="middle" font-size="%d" font-weight="%s" fill="%s">%s</text>`, x, startY+float64(index)*lineHeight, fontSize, weight, fill, svgEsc(line))
	}
}

func wrapTextLines(text string, maxWidth int, fontSize int, maxLines int) []string {
	text = strings.TrimSpace(text)
	if text == "" || maxLines <= 0 {
		return nil
	}
	maxUnits := maxInt(4, maxWidth*2/fontSize)
	runes := []rune(text)
	lines := []string{}
	current := []rune{}
	currentUnits := 0
	for _, r := range runes {
		unit := runeDisplayUnits(r)
		if currentUnits+unit > maxUnits && len(current) > 0 {
			lines = append(lines, strings.TrimSpace(string(current)))
			current = []rune{}
			currentUnits = 0
			if len(lines) == maxLines {
				break
			}
		}
		current = append(current, r)
		currentUnits += unit
	}
	if len(lines) < maxLines && len(current) > 0 {
		lines = append(lines, strings.TrimSpace(string(current)))
	}
	if len(lines) == maxLines && len([]rune(strings.Join(lines, ""))) < len(runes) {
		lines[maxLines-1] = compactText(lines[maxLines-1], maxInt(1, len([]rune(lines[maxLines-1]))-1))
	}
	return lines
}

func compactText(text string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(text))
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return string(runes)
	}
	if maxRunes == 1 {
		return "."
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

func textApproxWidth(text string, fontSize int) int {
	units := 0
	for _, r := range text {
		units += runeDisplayUnits(r)
	}
	return units * fontSize / 2
}

func runeDisplayUnits(r rune) int {
	if r <= 0x007f {
		return 1
	}
	return 2
}

func writeSVGStart(b *strings.Builder, width int, height int, p diagramPalette, title string) {
	fmt.Fprintf(b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height)
	fmt.Fprintf(b, `<rect width="100%%" height="100%%" fill="%s"/>`, p.Canvas)
	if strings.TrimSpace(title) != "" {
		fmt.Fprintf(b, `<text x="%d" y="44" text-anchor="middle" font-size="24" font-weight="800" fill="%s">%s</text>`, width/2, p.Text, svgEsc(title))
	}
}

func writeSVGEnd(b *strings.Builder, spec diagramSpec, p diagramPalette, y int) {
	if spec.Description != "" {
		fmt.Fprintf(b, `<text x="%d" y="%d" text-anchor="middle" font-size="13" fill="%s">%s</text>`, spec.Options.Width/2, y+24, p.Muted, svgEsc(spec.Description))
	}
	b.WriteString(`</svg>`)
}

func renderHTMLDocument(spec diagramSpec, svg string) string {
	title := html.EscapeString(firstNonEmpty(spec.Title, "Architecture Diagram"))
	return "<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n<title>" + title + "</title>\n<style>body{margin:0;background:#edebe1;font-family:Inter,-apple-system,BlinkMacSystemFont,'Segoe UI',Arial,sans-serif;color:#1a1a18;padding:32px;-webkit-font-smoothing:antialiased}.wrap{max-width:1280px;margin:0 auto}.diagram{background:#f5f3eb;border:1px solid #d4d1c7;border-radius:14px;overflow:auto;box-shadow:0 12px 40px rgba(26,26,24,.08),0 4px 12px rgba(26,26,24,.04)}svg{display:block;height:auto}.meta{margin-top:12px;color:#6b6961;font-size:13px;letter-spacing:.02em}@media(max-width:768px){body{padding:16px}.diagram{border-radius:10px}}</style>\n</head>\n<body>\n<div class=\"wrap\"><div class=\"diagram\">\n" + svg + "\n</div><div class=\"meta\">Type: " + html.EscapeString(spec.DiagramType) + "</div></div>\n</body>\n</html>\n"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
