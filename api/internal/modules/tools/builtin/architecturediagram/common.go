package architecturediagram

import (
	"encoding/json"
	"fmt"
	"html"
	"math"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
)

const (
	defaultDiagramFilename = "architecture-diagram"
	svgMimeType            = "image/svg+xml"
	htmlMimeType           = "text/html"
	maxDiagramFileBytes    = 2 * 1024 * 1024
)

var diagramFilenameUnsafePattern = regexp.MustCompile(`[^a-zA-Z0-9._\-\p{Han}]`)

type diagramNode struct {
	ID    string
	Label string
	Type  string
	Group string
	Layer string
}

type diagramEdge struct {
	From  string
	To    string
	Label string
}

type diagramGroup struct {
	ID    string
	Label string
}

type diagramSpec struct {
	DiagramType  string
	Title        string
	Description  string
	Nodes        []diagramNode
	Edges        []diagramEdge
	Groups       []diagramGroup
	Columns      []string
	Rows         []string
	Cells        [][]string
	Participants []string
	Messages     []diagramEdge
	States       []diagramNode
	Transitions  []diagramEdge
	Entities     []diagramEntity
	Relations    []diagramEdge
	Options      diagramOptions
}

type diagramEntity struct {
	ID     string
	Label  string
	Fields []string
}

type diagramOptions struct {
	Formats    []string
	Style      string
	Direction  string
	ShowLegend bool
	ShowLabels bool
	Width      int
	Height     int
}

type diagramRenderMeta struct {
	DiagramType string
	NodeCount   int
	EdgeCount   int
}

func normalizeDiagramType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "system", "system_architecture", "architecture", "microservice", "microservices", "frontend_backend", "api_gateway":
		return "system_architecture"
	case "agent", "ai_agent", "agent_architecture", "llm", "rag", "tool_calling":
		return "agent_architecture"
	case "dataflow", "data_flow", "dfd":
		return "data_flow"
	case "flow", "flowchart", "workflow", "process":
		return "flowchart"
	case "comparison", "matrix", "comparison_matrix", "feature_matrix":
		return "comparison_matrix"
	case "sequence", "sequence_diagram", "timing":
		return "sequence"
	case "state", "state_diagram", "state_machine":
		return "state"
	case "er", "erd", "entity_relationship", "entity_relationship_diagram":
		return "er"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func parseDiagramData(diagramType string, title string, description string, data map[string]interface{}, options map[string]interface{}) (diagramSpec, error) {
	if len(data) == 0 {
		return diagramSpec{}, fmt.Errorf("data is required")
	}
	parsedOptions, err := parseDiagramOptions(options)
	if err != nil {
		return diagramSpec{}, err
	}
	spec := diagramSpec{
		DiagramType: diagramType,
		Title:       strings.TrimSpace(title),
		Description: strings.TrimSpace(description),
		Options:     parsedOptions,
	}
	switch diagramType {
	case "system_architecture", "agent_architecture", "data_flow", "flowchart":
		nodes, err := parseDiagramNodes(data["nodes"], "data.nodes")
		if err != nil {
			return diagramSpec{}, err
		}
		edges, err := parseDiagramEdges(data["edges"], "data.edges")
		if err != nil {
			return diagramSpec{}, err
		}
		groups, err := parseDiagramGroups(data["groups"])
		if err != nil {
			return diagramSpec{}, err
		}
		if err := validateNodeEdges(nodes, edges); err != nil {
			return diagramSpec{}, err
		}
		spec.Nodes = nodes
		spec.Edges = edges
		spec.Groups = groups
	case "comparison_matrix":
		columns, err := stringSliceValue(data["columns"], "data.columns")
		if err != nil {
			return diagramSpec{}, err
		}
		rows, err := stringSliceValue(data["rows"], "data.rows")
		if err != nil {
			return diagramSpec{}, err
		}
		cells, err := matrixValue(data["cells"], len(rows), len(columns))
		if err != nil {
			return diagramSpec{}, err
		}
		spec.Columns = columns
		spec.Rows = rows
		spec.Cells = cells
	case "sequence":
		participants, err := stringSliceValue(data["participants"], "data.participants")
		if err != nil {
			return diagramSpec{}, err
		}
		messages, err := parseDiagramEdges(data["messages"], "data.messages")
		if err != nil {
			return diagramSpec{}, err
		}
		if err := validateNamedEdges(participants, messages, "participant"); err != nil {
			return diagramSpec{}, err
		}
		spec.Participants = participants
		spec.Messages = messages
	case "state":
		states, err := parseDiagramNodes(data["states"], "data.states")
		if err != nil {
			return diagramSpec{}, err
		}
		transitions, err := parseDiagramEdges(data["transitions"], "data.transitions")
		if err != nil {
			return diagramSpec{}, err
		}
		if err := validateNodeEdges(states, transitions); err != nil {
			return diagramSpec{}, err
		}
		spec.States = states
		spec.Transitions = transitions
	case "er":
		entities, err := parseDiagramEntities(data["entities"])
		if err != nil {
			return diagramSpec{}, err
		}
		relations, err := parseDiagramEdges(data["relationships"], "data.relationships")
		if err != nil {
			return diagramSpec{}, err
		}
		names := make([]string, 0, len(entities))
		for _, entity := range entities {
			names = append(names, entity.ID)
		}
		if err := validateNamedEdges(names, relations, "entity"); err != nil {
			return diagramSpec{}, err
		}
		spec.Entities = entities
		spec.Relations = relations
	default:
		return diagramSpec{}, fmt.Errorf("unsupported diagram_type: %s", diagramType)
	}
	return spec, nil
}

func parseDiagramOptions(raw map[string]interface{}) (diagramOptions, error) {
	width := intOption(raw, "width", 1200, 480, 2400)
	height := intOption(raw, "height", 760, 320, 1800)
	formats, err := formatList(raw["formats"])
	if err != nil {
		return diagramOptions{}, err
	}
	return diagramOptions{
		Formats:    formats,
		Style:      normalizeDiagramStyle(stringValue(raw["style"])),
		Direction:  normalizeDiagramDirection(stringValue(raw["direction"])),
		ShowLegend: boolOption(raw, "show_legend", true),
		ShowLabels: boolOption(raw, "show_labels", true),
		Width:      width,
		Height:     height,
	}, nil
}

func normalizeDiagramStyle(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "business", "technical", "presentation", "paper":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "simple"
	}
}

func normalizeDiagramDirection(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "top_to_bottom", "tb", "vertical":
		return "top_to_bottom"
	default:
		return "left_to_right"
	}
}

func formatList(raw interface{}) ([]string, error) {
	if raw == nil {
		return []string{"svg", "html"}, nil
	}
	formats := []string{}
	seen := map[string]struct{}{}
	add := func(value string) error {
		value = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(value), "."))
		switch value {
		case "svg", "html":
			if _, ok := seen[value]; !ok {
				seen[value] = struct{}{}
				formats = append(formats, value)
			}
			return nil
		case "":
			return fmt.Errorf("formats must include at least one supported format: svg or html")
		default:
			return fmt.Errorf("unsupported format: %s", value)
		}
	}
	switch value := raw.(type) {
	case []interface{}:
		for _, item := range value {
			if err := add(stringValue(item)); err != nil {
				return nil, err
			}
		}
	case []string:
		for _, item := range value {
			if err := add(item); err != nil {
				return nil, err
			}
		}
	case string:
		for _, item := range strings.Split(value, ",") {
			if err := add(item); err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("formats must be a string or array")
	}
	if len(formats) == 0 {
		return nil, fmt.Errorf("formats must include at least one supported format: svg or html")
	}
	return formats, nil
}

func parseDiagramNodes(raw interface{}, label string) ([]diagramNode, error) {
	items, ok := raw.([]interface{})
	if !ok || len(items) == 0 {
		return nil, fmt.Errorf("%s must contain at least 1 node", label)
	}
	nodes := make([]diagramNode, 0, len(items))
	seen := map[string]struct{}{}
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be an object", label, index)
		}
		id := strings.TrimSpace(stringValue(item["id"]))
		if id == "" {
			return nil, fmt.Errorf("%s[%d].id is required", label, index)
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("duplicate node id: %s", id)
		}
		seen[id] = struct{}{}
		node := diagramNode{
			ID:    id,
			Label: firstNonEmpty(stringValue(item["label"]), id),
			Type:  stringValue(item["type"]),
			Group: stringValue(item["group"]),
			Layer: stringValue(item["layer"]),
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func parseDiagramEdges(raw interface{}, label string) ([]diagramEdge, error) {
	items, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("%s must be an array", label)
	}
	edges := make([]diagramEdge, 0, len(items))
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be an object", label, index)
		}
		edge := diagramEdge{
			From:  strings.TrimSpace(stringValue(item["from"])),
			To:    strings.TrimSpace(stringValue(item["to"])),
			Label: strings.TrimSpace(stringValue(item["label"])),
		}
		if edge.From == "" || edge.To == "" {
			return nil, fmt.Errorf("%s[%d] requires from and to", label, index)
		}
		edges = append(edges, edge)
	}
	return edges, nil
}

func parseDiagramGroups(raw interface{}) ([]diagramGroup, error) {
	if raw == nil {
		return nil, nil
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("data.groups must be an array")
	}
	groups := make([]diagramGroup, 0, len(items))
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("data.groups[%d] must be an object", index)
		}
		id := strings.TrimSpace(stringValue(item["id"]))
		if id == "" {
			return nil, fmt.Errorf("data.groups[%d].id is required", index)
		}
		groups = append(groups, diagramGroup{ID: id, Label: firstNonEmpty(stringValue(item["label"]), id)})
	}
	return groups, nil
}

func parseDiagramEntities(raw interface{}) ([]diagramEntity, error) {
	items, ok := raw.([]interface{})
	if !ok || len(items) == 0 {
		return nil, fmt.Errorf("data.entities must contain at least 1 entity")
	}
	entities := make([]diagramEntity, 0, len(items))
	seen := map[string]struct{}{}
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("data.entities[%d] must be an object", index)
		}
		id := strings.TrimSpace(stringValue(item["id"]))
		if id == "" {
			return nil, fmt.Errorf("data.entities[%d].id is required", index)
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("duplicate entity id: %s", id)
		}
		seen[id] = struct{}{}
		fields, _ := stringSliceValue(item["fields"], "data.entities.fields")
		entities = append(entities, diagramEntity{
			ID:     id,
			Label:  firstNonEmpty(stringValue(item["label"]), id),
			Fields: fields,
		})
	}
	return entities, nil
}

func validateNodeEdges(nodes []diagramNode, edges []diagramEdge) error {
	ids := make([]string, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, node.ID)
	}
	return validateNamedEdges(ids, edges, "node")
}

func validateNamedEdges(names []string, edges []diagramEdge, noun string) error {
	seen := map[string]struct{}{}
	for _, name := range names {
		seen[name] = struct{}{}
	}
	for index, edge := range edges {
		if _, ok := seen[edge.From]; !ok {
			return fmt.Errorf("edge[%d].from references missing %s: %s", index, noun, edge.From)
		}
		if _, ok := seen[edge.To]; !ok {
			return fmt.Errorf("edge[%d].to references missing %s: %s", index, noun, edge.To)
		}
	}
	return nil
}

func matrixValue(raw interface{}, rowCount int, columnCount int) ([][]string, error) {
	if rowCount == 0 || columnCount == 0 {
		return nil, fmt.Errorf("data.rows and data.columns must be non-empty")
	}
	items, ok := raw.([]interface{})
	if !ok || len(items) != rowCount {
		return nil, fmt.Errorf("data.cells must contain %d rows", rowCount)
	}
	cells := make([][]string, 0, len(items))
	for rowIndex, rawRow := range items {
		rowItems, ok := rawRow.([]interface{})
		if !ok || len(rowItems) != columnCount {
			return nil, fmt.Errorf("data.cells[%d] must contain %d values", rowIndex, columnCount)
		}
		row := make([]string, 0, len(rowItems))
		for _, cell := range rowItems {
			row = append(row, stringValue(cell))
		}
		cells = append(cells, row)
	}
	return cells, nil
}

func mapParam(params map[string]interface{}, key string) (map[string]interface{}, error) {
	raw, ok := params[key]
	if !ok || raw == nil {
		return nil, fmt.Errorf("%s is required", key)
	}
	if value, ok := raw.(map[string]interface{}); ok {
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

func optionalMapParam(params map[string]interface{}, key string) (map[string]interface{}, error) {
	raw, ok := params[key]
	if !ok || raw == nil {
		return map[string]interface{}{}, nil
	}
	return mapParam(params, key)
}

func stringSliceValue(raw interface{}, label string) ([]string, error) {
	switch value := raw.(type) {
	case []interface{}:
		if len(value) == 0 {
			return nil, fmt.Errorf("%s must contain at least 1 item", label)
		}
		result := make([]string, 0, len(value))
		for _, item := range value {
			text := strings.TrimSpace(stringValue(item))
			if text != "" {
				result = append(result, text)
			}
		}
		if len(result) == 0 {
			return nil, fmt.Errorf("%s must contain at least 1 non-empty item", label)
		}
		return result, nil
	case []string:
		if len(value) == 0 {
			return nil, fmt.Errorf("%s must contain at least 1 item", label)
		}
		return append([]string(nil), value...), nil
	case string:
		text := strings.TrimSpace(value)
		if text == "" {
			return nil, fmt.Errorf("%s must contain at least 1 item", label)
		}
		if strings.HasPrefix(text, "[") {
			var values []string
			if err := json.Unmarshal([]byte(text), &values); err != nil {
				return nil, fmt.Errorf("%s must be an array, JSON string array, or CSV string", label)
			}
			return values, nil
		}
		parts := strings.Split(text, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) == 0 {
			return nil, fmt.Errorf("%s must contain at least 1 item", label)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("%s must be an array, JSON string array, or CSV string", label)
	}
}

func intOption(options map[string]interface{}, key string, fallback, minimum, maximum int) int {
	value, err := numberValue(options[key], key)
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

func rawStringParam(params map[string]interface{}, key string) string {
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
	if raw == nil {
		return ""
	}
	switch value := raw.(type) {
	case string:
		return value
	default:
		return fmt.Sprint(value)
	}
}

func svgEsc(value string) string {
	return html.EscapeString(value)
}

func buildDiagramFilename(raw string, extension string) string {
	name := sanitizeDiagramFilename(raw)
	if name == "" {
		name = defaultDiagramFilename
	}
	currentExt := filepath.Ext(name)
	if currentExt != "" {
		name = strings.TrimSuffix(name, currentExt)
	}
	return name + extension
}

func sanitizeDiagramFilename(raw string) string {
	name := strings.TrimSpace(filepath.Base(raw))
	if name == "." || name == string(filepath.Separator) {
		return ""
	}
	name = diagramFilenameUnsafePattern.ReplaceAllString(name, "_")
	name = strings.Trim(name, "._- ")
	runes := []rune(name)
	if len(runes) > 120 {
		name = string(runes[:120])
	}
	return name
}

func resolveDiagramFileLifecycle(raw string) (tool_file.ToolFileLifecycle, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "persistent":
		return tool_file.ToolFileLifecyclePersistent, nil
	case "temporary":
		return tool_file.ToolFileLifecycleTemporary, nil
	default:
		return "", fmt.Errorf("unsupported lifecycle: %s", raw)
	}
}

func appendDownloadQuery(rawURL string) string {
	if strings.Contains(rawURL, "?") {
		return rawURL + "&download=1"
	}
	return rawURL + "?download=1"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func sortedLayers(nodes []diagramNode) []string {
	seen := map[string]struct{}{}
	layers := []string{}
	for _, node := range nodes {
		layer := strings.TrimSpace(node.Layer)
		if layer == "" {
			layer = strings.TrimSpace(node.Group)
		}
		if layer == "" {
			layer = "default"
		}
		if _, ok := seen[layer]; !ok {
			seen[layer] = struct{}{}
			layers = append(layers, layer)
		}
	}
	return layers
}

func arrowPoint(x1, y1, x2, y2 float64) string {
	angle := math.Atan2(y2-y1, x2-x1)
	size := 9.0
	left := angle + math.Pi*0.82
	right := angle - math.Pi*0.82
	p1x := x2 + math.Cos(left)*size
	p1y := y2 + math.Sin(left)*size
	p2x := x2 + math.Cos(right)*size
	p2y := y2 + math.Sin(right)*size
	return fmt.Sprintf("%.1f,%.1f %.1f,%.1f %.1f,%.1f", x2, y2, p1x, p1y, p2x, p2y)
}
