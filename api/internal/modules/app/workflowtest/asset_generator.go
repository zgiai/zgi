package workflowtest

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/file_process/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin/filegenerator"
)

const (
	generatedAssetSourceKey = "__asset_source"
	generatedFixtureSpecKey = "__fixture_spec"
	generatedAssetSource    = "workflow_test_generated"
)

var generatedAssetFilenameUnsafe = regexp.MustCompile(`[^a-zA-Z0-9._\-\p{Han}]`)

type workflowTestAssetService struct {
	fileService interfaces.FileService
}

func newWorkflowTestAssetService(fileService interfaces.FileService) *workflowTestAssetService {
	if fileService == nil {
		return nil
	}
	return &workflowTestAssetService{fileService: fileService}
}

func (s *workflowTestAssetService) attachGeneratedAssets(ctx context.Context, req GenerateCasesRequest, organizationID string, item *GeneratedCase, index int) error {
	config := normalizeFileGenerationConfig(req.FileGeneration)
	if !config.Enabled {
		return nil
	}
	if s == nil || s.fileService == nil {
		return fmt.Errorf("workflow test file generation is not configured")
	}
	if strings.TrimSpace(req.AccountID) == "" {
		return fmt.Errorf("account id is required for workflow test file generation")
	}
	if strings.TrimSpace(organizationID) == "" {
		return fmt.Errorf("organization id is required for workflow test file generation")
	}
	fixtures := buildCaseFileFixtures(config, item, index)
	if len(fixtures) == 0 {
		return nil
	}
	if len(item.Turns) == 0 {
		item.Turns = []CaseTurn{{Role: "user", Content: strings.TrimSpace(item.Content)}}
	}
	if strings.TrimSpace(item.Turns[0].Role) == "" {
		item.Turns[0].Role = "user"
	}
	if item.Turns[0].Inputs == nil {
		item.Turns[0].Inputs = JSONMap{}
	}
	if strings.TrimSpace(item.Turns[0].Content) == "" {
		item.Turns[0].Content = strings.TrimSpace(item.Content)
	}

	uploaded := make([]map[string]interface{}, 0, len(fixtures))
	source := interfaces.FileSourceWorkflow
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	var workspaceIDPtr *string
	if workspaceID != "" {
		workspaceIDPtr = &workspaceID
	}
	for fixtureIndex, fixture := range fixtures {
		data, extension, mimeType, err := filegenerator.RenderGeneratedFile(renderFixtureContent(fixture), fixture.Format, fixture.Title)
		if err != nil {
			return fmt.Errorf("render workflow test fixture file: %w", err)
		}
		filename := buildGeneratedAssetFilename(fixture, extension, index, fixtureIndex)
		file, err := s.fileService.UploadFile(
			ctx,
			filename,
			data,
			mimeType,
			req.AccountID,
			organizationID,
			model.CreatedByRoleAccount,
			&source,
			workspaceIDPtr,
			false,
			false,
		)
		if err != nil {
			return fmt.Errorf("upload workflow test fixture file: %w", err)
		}
		item.Turns[0].Attachments = append(item.Turns[0].Attachments, CaseAttachment{
			Type:           attachmentTypeForFormat(extension),
			TransferMethod: "local_file",
			UploadFileID:   file.ID,
			Name:           file.Name,
		})
		uploaded = append(uploaded, map[string]interface{}{
			"upload_file_id": file.ID,
			"name":           file.Name,
			"format":         extension,
			"mime_type":      mimeType,
			"fixture":        fixture,
		})
	}
	item.Turns[0].Inputs[generatedAssetSourceKey] = generatedAssetSource
	item.Turns[0].Inputs[generatedFixtureSpecKey] = uploaded
	caseMode := normalizeCaseMode(req.CaseMode)
	if caseMode == "" {
		caseMode = "task"
	}
	item.Turns[0].Inputs[caseModeInputKey] = caseMode
	if caseMode == "conversation" {
		enrichGeneratedAssetConversationChecks(item, fixtures)
		return nil
	}
	if _, exists := item.Turns[0].Inputs[expectedChecksInputKey]; !exists {
		checks := expectedChecksFromFixtures(fixtures, item.ExpectedResult)
		if len(checks) > 0 {
			item.Turns[0].Inputs[expectedChecksInputKey] = map[string]interface{}{
				"output_contains": checks,
				"conditions": []TaskExpectedCheckCondition{{
					ID:        "check_output_contains_1",
					Type:      "output_contains",
					Operator:  "contains",
					Values:    checks,
					MatchMode: "semantic",
					Severity:  "critical",
					Source:    "ai_generated",
				}},
			}
		}
	}
	checks := expectedChecksFromInput(item.Turns[0].Inputs[expectedChecksInputKey])
	item.Turns[0].Inputs[evaluationSchemaInputKey] = buildGeneratedTaskEvaluationSchema(item, checks)
	return nil
}

func enrichGeneratedAssetConversationChecks(item *GeneratedCase, fixtures []GeneratedFileFixture) {
	if item == nil || len(item.Turns) == 0 {
		return
	}
	turn := &item.Turns[0]
	if turn.Inputs == nil {
		turn.Inputs = JSONMap{}
	}
	if !conversationExpectedChecksFromInput(turn.Inputs[turnChecksInputKey]).Useful() {
		checks := conversationChecksFromGeneratedAssets(fixtures, item.ExpectedResult)
		if checks.Useful() {
			turn.Inputs[turnChecksInputKey] = checks.JSONMap()
		}
	}
	if len(item.Turns) > 1 && !conversationExpectedChecksFromInput(turn.Inputs[conversationChecksInputKey]).Useful() {
		checks := defaultGlobalConversationChecks()
		if checks.Useful() {
			turn.Inputs[conversationChecksInputKey] = checks.JSONMap()
		}
	}
}

func conversationChecksFromGeneratedAssets(fixtures []GeneratedFileFixture, fallback string) ConversationExpectedChecks {
	values := expectedChecksFromFixtures(fixtures, fallback)
	if len(values) == 0 {
		return ConversationExpectedChecks{}
	}
	return normalizeConversationExpectedChecks(ConversationExpectedChecks{Conditions: []ConversationExpectedCheckCondition{{
		Type:      "task_completion",
		Operator:  "passed",
		Values:    values,
		MatchMode: "semantic",
		Severity:  "critical",
		Source:    "ai_generated",
	}}})
}

func normalizeFileGenerationConfig(config *FileGenerationConfig) FileGenerationConfig {
	if config == nil || !config.Enabled {
		return FileGenerationConfig{}
	}
	formats := normalizeFileGenerationList(config.Formats)
	if len(formats) == 0 {
		formats = []string{"docx"}
	}
	filesPerCase := config.FilesPerCase
	if filesPerCase <= 0 {
		filesPerCase = 1
	}
	if filesPerCase > 5 {
		filesPerCase = 5
	}
	return FileGenerationConfig{
		Enabled:      true,
		Formats:      formats,
		FilesPerCase: filesPerCase,
		Complexities: normalizeFileGenerationList(config.Complexities),
		ContentTypes: normalizeFileGenerationList(config.ContentTypes),
	}
}

func normalizeFileGenerationList(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func buildCaseFileFixtures(config FileGenerationConfig, item *GeneratedCase, caseIndex int) []GeneratedFileFixture {
	fixtures := normalizeGeneratedFileFixtures(item.FileFixtures)
	if len(fixtures) == 0 {
		fixtures = make([]GeneratedFileFixture, 0, config.FilesPerCase)
	}
	for len(fixtures) < config.FilesPerCase {
		format := config.Formats[len(fixtures)%len(config.Formats)]
		fixtures = append(fixtures, defaultGeneratedFileFixture(config, item, caseIndex, len(fixtures), format))
	}
	if len(fixtures) > config.FilesPerCase {
		fixtures = fixtures[:config.FilesPerCase]
	}
	for index := range fixtures {
		if strings.TrimSpace(fixtures[index].Format) == "" {
			fixtures[index].Format = config.Formats[index%len(config.Formats)]
		}
		if strings.TrimSpace(fixtures[index].Title) == "" {
			fixtures[index].Title = defaultFixtureTitle(item, caseIndex, index)
		}
		if strings.TrimSpace(fixtures[index].Content) == "" {
			fixtures[index].Content = defaultFixturePlainText(item, config, caseIndex, index)
		}
	}
	return fixtures
}

func defaultGeneratedFileFixture(config FileGenerationConfig, item *GeneratedCase, caseIndex int, fixtureIndex int, format string) GeneratedFileFixture {
	return GeneratedFileFixture{
		Format:         format,
		Title:          defaultFixtureTitle(item, caseIndex, fixtureIndex),
		Content:        defaultFixturePlainText(item, config, caseIndex, fixtureIndex),
		Description:    "Generated workflow test input file",
		Facts:          []string{strings.TrimSpace(item.Content), strings.TrimSpace(item.ExpectedResult)},
		ExpectedChecks: expectedChecksFromFixtures(nil, item.ExpectedResult),
	}
}

func defaultFixtureTitle(item *GeneratedCase, caseIndex int, fixtureIndex int) string {
	title := strings.TrimSpace(item.Content)
	if title == "" {
		title = fmt.Sprintf("workflow-test-case-%02d", caseIndex+1)
	}
	if runes := []rune(title); len(runes) > 48 {
		title = string(runes[:48])
	}
	if fixtureIndex > 0 {
		return fmt.Sprintf("%s-%d", title, fixtureIndex+1)
	}
	return title
}

func defaultFixturePlainText(item *GeneratedCase, config FileGenerationConfig, caseIndex int, fixtureIndex int) string {
	complexities := strings.Join(config.Complexities, ", ")
	if complexities == "" {
		complexities = "normal"
	}
	contentTypes := strings.Join(config.ContentTypes, ", ")
	if contentTypes == "" {
		contentTypes = "document"
	}
	return strings.TrimSpace(fmt.Sprintf(`标题：%s
内容类型：%s
复杂度：%s

任务输入：
%s

关键事实：
%s

预期检查点：
%s
`, defaultFixtureTitle(item, caseIndex, fixtureIndex), contentTypes, complexities, strings.TrimSpace(item.Content), strings.TrimSpace(item.ExpectedResult), strings.TrimSpace(item.ExpectedResult)))
}

func renderFixtureContent(fixture GeneratedFileFixture) string {
	format := strings.ToLower(strings.TrimSpace(fixture.Format))
	content := strings.TrimSpace(fixture.Content)
	if content == "" {
		content = strings.TrimSpace(fixture.Description)
	}
	switch format {
	case "csv", "xlsx", "excel":
		if looksLikeCSV(content) {
			return content
		}
		return fixtureCSVContent(fixture)
	case "json":
		if json.Valid([]byte(content)) {
			return content
		}
		data, _ := json.Marshal(map[string]interface{}{
			"title":           fixture.Title,
			"content":         content,
			"facts":           fixture.Facts,
			"expected_checks": fixture.ExpectedChecks,
		})
		return string(data)
	default:
		return content
	}
}

func looksLikeCSV(content string) bool {
	reader := csv.NewReader(strings.NewReader(content))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	return err == nil && len(records) > 0 && len(records[0]) > 1
}

func fixtureCSVContent(fixture GeneratedFileFixture) string {
	var builder strings.Builder
	writer := csv.NewWriter(&builder)
	_ = writer.Write([]string{"field", "value"})
	_ = writer.Write([]string{"title", strings.TrimSpace(fixture.Title)})
	_ = writer.Write([]string{"content", strings.TrimSpace(fixture.Content)})
	for index, fact := range fixture.Facts {
		_ = writer.Write([]string{fmt.Sprintf("fact_%d", index+1), fact})
	}
	for index, check := range fixture.ExpectedChecks {
		_ = writer.Write([]string{fmt.Sprintf("expected_check_%d", index+1), check})
	}
	writer.Flush()
	return builder.String()
}

func buildGeneratedAssetFilename(fixture GeneratedFileFixture, extension string, caseIndex int, fixtureIndex int) string {
	name := strings.TrimSpace(fixture.Filename)
	if name == "" {
		name = fmt.Sprintf("workflow-test-case-%02d-file-%02d", caseIndex+1, fixtureIndex+1)
	}
	name = filepath.Base(name)
	name = generatedAssetFilenameUnsafe.ReplaceAllString(name, "-")
	name = strings.Trim(name, ".- ")
	if name == "" {
		name = fmt.Sprintf("workflow-test-case-%02d-file-%02d", caseIndex+1, fixtureIndex+1)
	}
	extension = strings.TrimPrefix(strings.TrimSpace(extension), ".")
	if extension == "" {
		extension = "txt"
	}
	if strings.EqualFold(filepath.Ext(name), "."+extension) {
		return name
	}
	return strings.TrimSuffix(name, filepath.Ext(name)) + "." + extension
}

func attachmentTypeForFormat(format string) string {
	switch strings.ToLower(strings.TrimPrefix(format, ".")) {
	case "jpg", "jpeg", "png", "webp", "gif":
		return "image"
	default:
		return "document"
	}
}

func expectedChecksFromFixtures(fixtures []GeneratedFileFixture, fallback string) []string {
	checks := make([]string, 0)
	for _, fixture := range fixtures {
		checks = append(checks, fixture.ExpectedChecks...)
		if len(fixture.ExpectedChecks) == 0 {
			checks = append(checks, fixture.Facts...)
		}
	}
	if len(checks) == 0 {
		for _, item := range strings.Split(fallback, "\n") {
			item = strings.TrimSpace(strings.Trim(item, "，。；;,. "))
			if item != "" {
				checks = append(checks, item)
			}
			if len(checks) >= 5 {
				break
			}
		}
	}
	return normalizeGeneratedFixtureExpectedChecks(checks)
}

func normalizeGeneratedFixtureExpectedChecks(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}
