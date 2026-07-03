package architecturediagram

import (
	"context"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/config"
	workflowtoolfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/pkg/storage"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRenderNodeEdgeDiagramProducesSVGAndHTML(t *testing.T) {
	spec, err := parseDiagramData(
		"agent_architecture",
		"RAG Agent Architecture",
		"Agent retrieves context before answering.",
		map[string]interface{}{
			"nodes": []interface{}{
				map[string]interface{}{"id": "user", "label": "User", "type": "actor", "layer": "input"},
				map[string]interface{}{"id": "agent", "label": "Agent", "type": "orchestrator", "layer": "agent"},
				map[string]interface{}{"id": "vector", "label": "Vector Store", "type": "memory", "layer": "memory"},
				map[string]interface{}{"id": "llm", "label": "LLM", "type": "model", "layer": "model"},
			},
			"edges": []interface{}{
				map[string]interface{}{"from": "user", "to": "agent", "label": "query"},
				map[string]interface{}{"from": "agent", "to": "vector", "label": "retrieve"},
				map[string]interface{}{"from": "agent", "to": "llm", "label": "prompt"},
			},
		},
		map[string]interface{}{"style": "technical"},
	)
	require.NoError(t, err)
	svg, htmlDoc, meta, err := renderDiagram(spec)
	require.NoError(t, err)
	require.Equal(t, diagramRenderMeta{DiagramType: "agent_architecture", NodeCount: 4, EdgeCount: 3}, meta)
	require.True(t, strings.HasPrefix(svg, `<svg `))
	require.Contains(t, svg, "RAG Agent Architecture")
	require.Contains(t, svg, "Vector Store")
	require.Contains(t, htmlDoc, "<!doctype html>")
	require.Contains(t, htmlDoc, svg)
}

func TestRenderNodeEdgeDiagramExpandsCanvasForManyLayers(t *testing.T) {
	nodes := make([]interface{}, 0, 18)
	edges := make([]interface{}, 0, 17)
	for index := 0; index < 18; index++ {
		id := "node-" + strconv.Itoa(index)
		nodes = append(nodes, map[string]interface{}{"id": id, "label": "Node " + strconv.Itoa(index), "layer": strconv.Itoa(index)})
		if index > 0 {
			edges = append(edges, map[string]interface{}{"from": "node-" + strconv.Itoa(index-1), "to": id})
		}
	}
	spec, err := parseDiagramData(
		"system_architecture",
		"Large Architecture",
		"",
		map[string]interface{}{"nodes": nodes, "edges": edges},
		map[string]interface{}{"width": 1200, "height": 760},
	)
	require.NoError(t, err)

	svg, htmlDoc, _, err := renderDiagram(spec)
	require.NoError(t, err)
	match := regexp.MustCompile(`width="([0-9]+)"`).FindStringSubmatch(svg)
	require.Len(t, match, 2)
	width, err := strconv.Atoi(match[1])
	require.NoError(t, err)
	require.Greater(t, width, 4000)
	require.NotContains(t, htmlDoc, "max-width:100%")
}

func TestRenderNodeEdgeDiagramRoutesEdgesAndWrapsLongLabels(t *testing.T) {
	longLabel := "升级 / 人工复核 (高紧急/合同/投诉/退款)"
	spec, err := parseDiagramData(
		"agent_architecture",
		"Ticket Agent",
		"",
		map[string]interface{}{
			"nodes": []interface{}{
				map[string]interface{}{"id": "classify", "label": "问题分类", "type": "service", "layer": "1"},
				map[string]interface{}{"id": "review", "label": longLabel, "type": "output", "layer": "2"},
			},
			"edges": []interface{}{
				map[string]interface{}{"from": "classify", "to": "review", "label": "高紧急升级"},
			},
		},
		map[string]interface{}{"style": "technical"},
	)
	require.NoError(t, err)

	svg, _, _, err := renderDiagram(spec)
	require.NoError(t, err)
	require.Contains(t, svg, "<path ")
	require.NotContains(t, svg, `<line x1=`)
	require.NotContains(t, svg, longLabel)
	require.Contains(t, svg, "升级 / 人工复核")
	require.Contains(t, svg, "高紧急")
}

func TestRenderNodeEdgeDiagramOmitsNilLabelsAndMarksSharedTargets(t *testing.T) {
	spec, err := parseDiagramData(
		"system_architecture",
		"Shared Target",
		"",
		map[string]interface{}{
			"nodes": []interface{}{
				map[string]interface{}{"id": "a", "label": "A", "type": nil, "layer": "1"},
				map[string]interface{}{"id": "b", "label": "B", "layer": "1"},
				map[string]interface{}{"id": "c", "label": "C", "layer": "2"},
				map[string]interface{}{"id": "d", "label": "D", "layer": "3"},
			},
			"edges": []interface{}{
				map[string]interface{}{"from": "a", "to": "c", "label": nil},
				map[string]interface{}{"from": "b", "to": "c"},
				map[string]interface{}{"from": "c", "to": "d", "label": "next"},
			},
		},
		map[string]interface{}{"style": "technical"},
	)
	require.NoError(t, err)

	svg, _, _, err := renderDiagram(spec)
	require.NoError(t, err)
	require.NotContains(t, svg, "&lt;nil&gt;")
	require.NotContains(t, svg, ">nil<")
	require.Contains(t, svg, "<circle ")
	require.Contains(t, svg, "<polygon ")
}

func TestRenderNodeEdgeDiagramDrawsSemanticGroups(t *testing.T) {
	spec, err := parseDiagramData(
		"system_architecture",
		"Grouped Architecture",
		"",
		map[string]interface{}{
			"groups": []interface{}{
				map[string]interface{}{"id": "frontend", "label": "Frontend"},
				map[string]interface{}{"id": "backend", "label": "Backend"},
			},
			"nodes": []interface{}{
				map[string]interface{}{"id": "web", "label": "Web App", "group": "frontend", "layer": "1"},
				map[string]interface{}{"id": "api", "label": "API Service", "group": "backend", "layer": "2"},
				map[string]interface{}{"id": "db", "label": "Database", "group": "backend", "layer": "3"},
			},
			"edges": []interface{}{
				map[string]interface{}{"from": "web", "to": "api"},
				map[string]interface{}{"from": "api", "to": "db"},
			},
		},
		map[string]interface{}{"style": "technical"},
	)
	require.NoError(t, err)

	svg, _, _, err := renderDiagram(spec)
	require.NoError(t, err)
	require.Contains(t, svg, ">Frontend<")
	require.Contains(t, svg, ">Backend<")
	require.Contains(t, svg, `stroke-dasharray="7 5"`)
	require.Contains(t, svg, `fill="#eff6ff"`)
	require.Contains(t, svg, `fill="#ecfdf5"`)
}

func TestRenderNodeEdgeDiagramUsesPaperStyleLegendAndCardAccents(t *testing.T) {
	spec, err := parseDiagramData(
		"system_architecture",
		"Paper Architecture",
		"",
		map[string]interface{}{
			"groups": []interface{}{
				map[string]interface{}{"id": "frontend", "label": "Frontend"},
				map[string]interface{}{"id": "backend", "label": "Backend"},
			},
			"nodes": []interface{}{
				map[string]interface{}{"id": "web", "label": "Web", "type": "frontend", "group": "frontend", "layer": "1"},
				map[string]interface{}{"id": "api", "label": "API", "type": "service", "group": "backend", "layer": "2"},
			},
			"edges": []interface{}{
				map[string]interface{}{"from": "web", "to": "api"},
			},
		},
		map[string]interface{}{"style": "paper"},
	)
	require.NoError(t, err)

	svg, htmlDoc, _, err := renderDiagram(spec)
	require.NoError(t, err)
	require.Contains(t, svg, `fill="#edebe1"`)
	require.Contains(t, svg, `fill="#f5f3eb"`)
	require.Contains(t, svg, `fill="#3d5af1"`)
	require.Contains(t, svg, ">Frontend<")
	require.Contains(t, htmlDoc, "background:#edebe1")
	require.Contains(t, htmlDoc, "border-radius:14px")
}

func TestGroupBoxesKeepModerateGap(t *testing.T) {
	spec, err := parseDiagramData(
		"system_architecture",
		"Grouped Gap",
		"",
		map[string]interface{}{
			"groups": []interface{}{
				map[string]interface{}{"id": "frontend", "label": "Frontend"},
				map[string]interface{}{"id": "backend", "label": "Backend"},
			},
			"nodes": []interface{}{
				map[string]interface{}{"id": "web", "label": "Web", "group": "frontend", "layer": "1"},
				map[string]interface{}{"id": "api", "label": "API", "group": "backend", "layer": "1"},
			},
			"edges": []interface{}{
				map[string]interface{}{"from": "web", "to": "api"},
			},
		},
		nil,
	)
	require.NoError(t, err)

	layerBuckets := [][]diagramNode{{spec.Nodes[0], spec.Nodes[1]}}
	sortLayerBucketsByGroup(layerBuckets, spec)
	positions := map[string]point{
		layerBuckets[0][0].ID: {X: 110, Y: 110},
		layerBuckets[0][1].ID: {X: 110, Y: 262},
	}
	boxes := groupBoxesForNodes(spec, positions, 220, 88)
	require.Len(t, boxes, 2)
	gap := boxes[1].MinY - boxes[0].MaxY
	require.GreaterOrEqual(t, gap, 16.0)
	require.LessOrEqual(t, gap, 40.0)
}

func TestRenderNodeEdgeDiagramDoesNotGroupNumericLayers(t *testing.T) {
	spec, err := parseDiagramData(
		"flowchart",
		"Numeric Layers",
		"",
		map[string]interface{}{
			"nodes": []interface{}{
				map[string]interface{}{"id": "a", "label": "A", "layer": "1"},
				map[string]interface{}{"id": "b", "label": "B", "layer": "2"},
			},
			"edges": []interface{}{
				map[string]interface{}{"from": "a", "to": "b"},
			},
		},
		nil,
	)
	require.NoError(t, err)

	svg, _, _, err := renderDiagram(spec)
	require.NoError(t, err)
	require.NotContains(t, svg, `stroke-dasharray="7 5"`)
	require.NotContains(t, svg, ">1<")
	require.NotContains(t, svg, ">2<")
}

func TestRenderSpecializedDiagramTypes(t *testing.T) {
	tests := []struct {
		name        string
		diagramType string
		data        map[string]interface{}
		want        string
	}{
		{
			name:        "comparison matrix",
			diagramType: "comparison_matrix",
			data: map[string]interface{}{
				"columns": []interface{}{"Option A", "Option B"},
				"rows":    []interface{}{"Cost", "Ops"},
				"cells": []interface{}{
					[]interface{}{"Low", "High"},
					[]interface{}{"Simple", "Managed"},
				},
			},
			want: "Option A",
		},
		{
			name:        "sequence",
			diagramType: "sequence",
			data: map[string]interface{}{
				"participants": []interface{}{"Client", "API", "Service"},
				"messages": []interface{}{
					map[string]interface{}{"from": "Client", "to": "API", "label": "POST /orders"},
					map[string]interface{}{"from": "API", "to": "Service", "label": "Create order"},
				},
			},
			want: "POST /orders",
		},
		{
			name:        "state",
			diagramType: "state",
			data: map[string]interface{}{
				"states": []interface{}{
					map[string]interface{}{"id": "created", "label": "Created", "layer": "1"},
					map[string]interface{}{"id": "paid", "label": "Paid", "layer": "2"},
				},
				"transitions": []interface{}{
					map[string]interface{}{"from": "created", "to": "paid", "label": "payment"},
				},
			},
			want: "Created",
		},
		{
			name:        "er",
			diagramType: "er",
			data: map[string]interface{}{
				"entities": []interface{}{
					map[string]interface{}{"id": "users", "label": "users", "fields": []interface{}{"id PK", "email"}},
					map[string]interface{}{"id": "orders", "label": "orders", "fields": []interface{}{"id PK", "user_id FK"}},
				},
				"relationships": []interface{}{
					map[string]interface{}{"from": "users", "to": "orders", "label": "1:N"},
				},
			},
			want: "user_id FK",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := parseDiagramData(tt.diagramType, tt.name, "", tt.data, nil)
			require.NoError(t, err)
			svg, _, _, err := renderDiagram(spec)
			require.NoError(t, err)
			require.Contains(t, svg, tt.want)
		})
	}
}

func TestParseDiagramDataRejectsInvalidReferences(t *testing.T) {
	_, err := parseDiagramData(
		"system_architecture",
		"",
		"",
		map[string]interface{}{
			"nodes": []interface{}{
				map[string]interface{}{"id": "api", "label": "API"},
			},
			"edges": []interface{}{
				map[string]interface{}{"from": "api", "to": "missing", "label": "calls"},
			},
		},
		nil,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing node")
}

func TestParseDiagramDataRejectsExplicitUnsupportedFormats(t *testing.T) {
	_, err := parseDiagramData(
		"system_architecture",
		"",
		"",
		map[string]interface{}{
			"nodes": []interface{}{
				map[string]interface{}{"id": "api", "label": "API"},
			},
			"edges": []interface{}{},
		},
		map[string]interface{}{"formats": []interface{}{"png"}},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported format: png")
}

func TestParseDiagramDataDefaultsFormatsOnlyWhenMissing(t *testing.T) {
	spec, err := parseDiagramData(
		"system_architecture",
		"",
		"",
		map[string]interface{}{
			"nodes": []interface{}{
				map[string]interface{}{"id": "api", "label": "API"},
			},
			"edges": []interface{}{},
		},
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, []string{"svg", "html"}, spec.Options.Formats)
}

func TestSanitizeDiagramFilenameTruncatesByRune(t *testing.T) {
	name := sanitizeDiagramFilename(strings.Repeat("架", 130))
	require.True(t, utf8.ValidString(name))
	require.Len(t, []rune(name), 120)
}

func TestGenerateArchitectureDiagramToolReturnsSVGAndHTMLMetadata(t *testing.T) {
	db, mock, cleanup := openArchitectureDiagramMockDB(t)
	defer cleanup()
	for range 2 {
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO "tool_files"`).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
	}

	oldManager := workflowtoolfile.GlobalToolFileManager
	oldSignature := workflowtoolfile.GlobalFileSignature
	t.Cleanup(func() {
		workflowtoolfile.GlobalToolFileManager = oldManager
		workflowtoolfile.GlobalFileSignature = oldSignature
	})

	fileStorage := newArchitectureMemoryStorage()
	workflowtoolfile.GlobalToolFileManager = workflowtoolfile.NewToolFileManager(db, fileStorage)
	workflowtoolfile.GlobalFileSignature = workflowtoolfile.NewFileSignature(&config.Config{
		App: config.AppConfig{
			SecretKey:          "test-secret-key",
			FilesURL:           "http://files.example.test",
			FilesAccessTimeout: 3600,
		},
	})

	messages, err := NewGenerateArchitectureDiagramTool("tenant-1").Invoke(
		context.Background(),
		"user-1",
		map[string]interface{}{
			"diagram_type":    "system_architecture",
			"title":           "Order Platform",
			"output_filename": "order-platform",
			"data": map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{"id": "web", "label": "Web App", "layer": "client"},
					map[string]interface{}{"id": "api", "label": "API", "layer": "services"},
					map[string]interface{}{"id": "db", "label": "PostgreSQL", "layer": "data"},
				},
				"edges": []interface{}{
					map[string]interface{}{"from": "web", "to": "api", "label": "HTTPS"},
					map[string]interface{}{"from": "api", "to": "db", "label": "SQL"},
				},
			},
		},
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)
	require.Len(t, messages, 3)
	require.Equal(t, tools.ToolInvokeMessageTypeFile, messages[0].Type)
	require.Equal(t, tools.ToolInvokeMessageTypeFile, messages[1].Type)
	require.Equal(t, tools.ToolInvokeMessageTypeJSON, messages[2].Type)

	jsonPayload := messages[2].Data
	require.Equal(t, "system_architecture", jsonPayload["diagram_type"])
	require.Equal(t, "order-platform.svg", jsonPayload["filename"])
	require.Equal(t, "svg", jsonPayload["format"])
	require.Equal(t, svgMimeType, jsonPayload["mime_type"])
	require.NotEmpty(t, jsonPayload["file_id"])
	require.NotEmpty(t, jsonPayload["url"])
	require.NotEmpty(t, jsonPayload["download_url"])
	parsed, err := url.Parse(jsonPayload["download_url"].(string))
	require.NoError(t, err)
	require.Equal(t, "1", parsed.Query().Get("download"))

	files, ok := jsonPayload["files"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, files, 2)
	require.Equal(t, "html", files[1]["format"])
	require.Equal(t, htmlMimeType, files[1]["mime_type"])
	require.Len(t, fileStorage.files, 2)
	require.NoError(t, mock.ExpectationsWereMet())
}

type architectureMemoryStorage struct {
	files map[string][]byte
}

func newArchitectureMemoryStorage() *architectureMemoryStorage {
	return &architectureMemoryStorage{files: make(map[string][]byte)}
}

func (s *architectureMemoryStorage) Save(filename string, data []byte) error {
	s.files[filename] = append([]byte(nil), data...)
	return nil
}

func (s *architectureMemoryStorage) Load(filename string) ([]byte, error) {
	data, ok := s.files[filename]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

func (s *architectureMemoryStorage) LoadStream(filename string) (<-chan []byte, error) {
	data, err := s.Load(filename)
	if err != nil {
		return nil, err
	}
	ch := make(chan []byte, 1)
	ch <- data
	close(ch)
	return ch, nil
}

func (s *architectureMemoryStorage) Download(filename string, targetPath string) error {
	return nil
}

func (s *architectureMemoryStorage) Exists(filename string) (bool, error) {
	_, ok := s.files[filename]
	return ok, nil
}

func (s *architectureMemoryStorage) Delete(filename string) error {
	delete(s.files, filename)
	return nil
}

func (s *architectureMemoryStorage) List(prefix string) ([]storage.FileInfo, error) {
	return nil, nil
}

func openArchitectureDiagramMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	require.NoError(t, err)
	mock.MatchExpectationsInOrder(false)

	return db, mock, func() {
		_ = sqlDB.Close()
	}
}
