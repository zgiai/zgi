package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/dto"
	workflowfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/tokenestimate"
)

func TestAttachmentPreviewsBoundCurrentAndHistoricalContext(t *testing.T) {
	bundle := &attachmentBundle{}
	for i := range 4 {
		file := attachmentFile{ID: fmt.Sprintf("file-%d", i), Name: "report.txt", MimeType: "text/plain"}
		file.applyExtractedContent(strings.Repeat("汉🙂", 5000)+"unseen-tail", false)
		bundle.Files = append(bundle.Files, file)
	}
	files := bundle.promptFiles()
	var count int
	for _, file := range files {
		if runeCount(file.Content) > 4000 {
			t.Fatal("per-file preview exceeded 4000 Unicode characters")
		}
		count += runeCount(file.Content)
	}
	if count != 12000 || files[3].Content != "" || !strings.Contains(bundle.previewContentSections(), "File ID: file-3") {
		t.Fatal("total preview budget discarded file metadata or exceeded 12000 characters")
	}
	svc := &service{tokenEstimator: tokenestimate.NewEstimator()}
	parts := &chatRequestParts{Query: "Summarize", Attachments: bundle}
	current, metadata, _ := svc.buildBudgetedCurrentUserContent(parts, "system", &budgetComputation{PromptBudget: 1_000_000}, 0)
	if strings.Contains(current.(string), "unseen-tail") || metadata["attachments_truncated"] != true {
		t.Fatal("large model budget injected full attachment text")
	}
	fallback, fallbackMetadata := svc.buildFallbackCurrentUserContent(parts)
	if current != fallback || fallbackMetadata["attachments_truncated"] != true {
		t.Fatal("fallback and budgeted preview policies differ")
	}
	history, err := svc.historicalUserMessage(t.Context(), &runtimemodel.Message{Query: "Summarize", Metadata: map[string]interface{}{"files": bundle.metadataFiles()}}, false)
	if err != nil || history.Content != current {
		t.Fatalf("history preview differs: %v", err)
	}
	sections, truncated := svc.fitAttachmentSectionsToBudget(parts, "system", 1)
	if !truncated || !strings.Contains(sections, "File ID: file-3") || strings.Contains(sections, "汉🙂") {
		t.Fatal("tight budget must remove text, not attachment references")
	}
	if bundle.Files[0].Content != strings.Repeat("汉🙂", 5000)+"unseen-tail" {
		t.Fatal("prompt projection mutated the original extracted text")
	}
}

func TestAttachmentPreviewPreservesShortSourceAndReclampsLegacyMetadata(t *testing.T) {
	short := attachmentFile{ID: "short"}
	short.applyExtractedContent("  原文🙂\n", false)
	if got := (&attachmentBundle{Files: []attachmentFile{short}}).promptFiles()[0].Content; got != short.Content {
		t.Fatalf("source whitespace changed: %q", got)
	}
	legacy := attachmentBundleFromMessageMetadata(map[string]interface{}{"files": []map[string]interface{}{{
		"id": "old", "content_preview": strings.Repeat("字", 8000), "content_chars": 9000,
	}}})
	if file := legacy.promptFiles()[0]; runeCount(file.Content) != 4000 || file.ContentChars != 9000 {
		t.Fatal("old previews were not bounded at projection time")
	}
}

type attachmentPageFiles struct {
	fakeRuntimeAttachmentFileService
	file *dto.UploadFile
}

func (f *attachmentPageFiles) GetFileByID(_ context.Context, id string) (*dto.UploadFile, error) {
	if f.file == nil || f.file.ID != id {
		return nil, errors.New("private storage diagnostic")
	}
	return f.file, nil
}

type attachmentPageExtractor struct {
	text  string
	calls int
	err   error
}

func (e *attachmentPageExtractor) ExtractMultipleFiles(_ context.Context, ids []string, _ workflowfile.ContentExtractionScope) ([]*workflowfile.FileContent, error) {
	e.calls++
	return []*workflowfile.FileContent{{FileID: ids[0], Content: e.text, FromCache: true}}, e.err
}

func attachmentReaderFixture(t *testing.T) (*service, *PreparedChat, *attachmentPageFiles, *attachmentPageExtractor) {
	t.Helper()
	owner := uuid.New()
	files := &attachmentPageFiles{file: &dto.UploadFile{ID: "file-1", Name: "source.txt", Size: 50000, IsTemporary: true, CreatedBy: owner.String()}}
	extractor := &attachmentPageExtractor{text: "  " + strings.Repeat("汉🙂", 7000) + "END\n"}
	parts := &chatRequestParts{FunctionCallingKnown: true, ModelSupportsFunctionCalling: true, AttachmentReadAllowed: true,
		ExecutionMode: executionModeDirectChat, Attachments: &attachmentBundle{Files: []attachmentFile{newPendingAttachmentFile(files.file)}}}
	registerAttachmentReadFiles(parts, nil)
	return &service{fileService: files, contentExtractor: extractor}, &PreparedChat{Scope: Scope{OrganizationID: uuid.New(), AccountID: owner}, Caller: Caller{Type: "aichat"}, parts: parts}, files, extractor
}

func TestImageAttachmentWithoutVisionRetainsMetadataWithoutVisualPayload(t *testing.T) {
	svc, prepared, files, extractor := attachmentReaderFixture(t)
	prepared.Conversation = &runtimemodel.Conversation{ID: uuid.New()}
	prepared.Message = &runtimemodel.Message{ID: uuid.New()}
	files.file.Name, files.file.Extension, files.file.MimeType = "source.png", "png", "image/png"
	prepared.parts.Attachments.Files = []attachmentFile{newPendingAttachmentFile(files.file)}
	if err := svc.extractPreparedAttachments(t.Context(), prepared, nil); err != nil {
		t.Fatal(err)
	}
	prepared.parts.Query = "resize this image"
	content, _ := svc.buildFallbackCurrentUserContent(prepared.parts)
	text, ok := content.(string)
	if !ok || !strings.Contains(text, "source.png") || !strings.Contains(text, "file-1") || !strings.Contains(text, "not") {
		t.Fatalf("non-visual file context = %#v", content)
	}
	if extractor.calls != 0 || prepared.parts.Attachments.Files[0].ImageURL != "" {
		t.Fatal("non-visual image triggered parsing or visual input")
	}
	if len(prepared.RuntimeAttachmentRefs()) != 1 {
		t.Fatal("image original was removed from authorized file operations")
	}
}

func TestMissingDocumentExtractorPreservesOriginalReference(t *testing.T) {
	svc, prepared, _, _ := attachmentReaderFixture(t)
	prepared.Conversation = &runtimemodel.Conversation{ID: uuid.New()}
	prepared.Message = &runtimemodel.Message{ID: uuid.New()}
	svc.contentExtractor = nil
	if err := svc.extractPreparedAttachments(t.Context(), prepared, nil); err != nil {
		t.Fatal(err)
	}
	if prepared.parts.Attachments.Files[0].FilteredReason != "text_extraction_unavailable" || len(prepared.RuntimeAttachmentRefs()) != 1 {
		t.Fatal("raw-only document lost its original file reference")
	}
}

func TestHistoricalImagesRecheckCurrentModelAndFormat(t *testing.T) {
	svc := &service{fileService: &fakeRuntimeAttachmentFileService{content: []byte("png-bytes")}}
	for _, test := range []struct {
		name, extension string
		vision          bool
		wantParts       int
	}{
		{"switch to non-visual", "png", false, 0},
		{"switch to visual", "png", true, 1},
		{"legacy unsupported format", "svg", true, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			file := attachmentFile{ID: "file-1", Name: "image." + test.extension, Extension: test.extension, Kind: attachmentKindImage}
			file.applyFiltered(attachmentFilteredReasonModelWithoutVision)
			bundle := &attachmentBundle{Files: []attachmentFile{file}}
			parts, err := svc.historicalImageParts(t.Context(), bundle, test.vision)
			if err != nil || len(parts) != test.wantParts {
				t.Fatalf("visual parts = %d, error = %v; want %d", len(parts), err, test.wantParts)
			}
			text := bundle.previewContentSections()
			if !strings.Contains(text, "File ID: file-1") || (test.wantParts == 0 && !strings.Contains(text, "NOT been read")) {
				t.Fatalf("incorrect historical image metadata: %s", text)
			}
		})
	}
}

func TestAttachmentReaderPagesBeyondOldLimitWithoutLinuxOrSkills(t *testing.T) {
	svc, prepared, _, extractor := attachmentReaderFixture(t)
	tools := svc.attachmentRuntimeTools(prepared)
	if len(tools) != 1 || !prepared.toolLoopEnabled() || chatPartsBusinessSkillsEnabled(prepared.parts) {
		t.Fatal("attachment reader requires Linux or business Skills")
	}
	var body strings.Builder
	var version string
	offset := 0
	for {
		args, _ := json.Marshal(map[string]interface{}{"file_id": "file-1", "offset": offset, "limit": 4000, "content_version": version})
		result := tools[0].Handler(t.Context(), adapter.ToolCall{Function: adapter.FunctionCall{Arguments: string(args)}})
		if result.Error != nil {
			t.Fatal(result.Error)
		}
		if _, exists := result.Result["content"]; exists {
			t.Fatal("source text leaked into persisted trace")
		}
		body.WriteString(result.ModelResult["content"].(string))
		version = result.ModelResult["content_version"].(string)
		offset = result.ModelResult["next_offset"].(int)
		if result.ModelResult["has_more"] == false {
			break
		}
	}
	if body.String() != extractor.text || offset <= 12000 {
		t.Fatal("pagination lost characters or stopped at old 12000 character limit")
	}
	extractor.text += "changed"
	args, _ := json.Marshal(map[string]interface{}{"file_id": "file-1", "content_version": version})
	if result := tools[0].Handler(t.Context(), adapter.ToolCall{Function: adapter.FunctionCall{Arguments: string(args)}}); result.Error == nil {
		t.Fatal("changed extracted version accepted")
	}
}

func TestAttachmentReaderRejectsInvalidAndRevokedReadsBeforeExtraction(t *testing.T) {
	svc, prepared, files, extractor := attachmentReaderFixture(t)
	tool := svc.attachmentRuntimeTools(prepared)[0]
	for _, args := range []string{`{"file_id":"other"}`, `{"file_id":"file-1","offset":-1}`, `{"file_id":"file-1","offset":1.5}`, `{"file_id":"file-1","limit":0}`, `{"file_id":"file-1","limit":12001}`, `{"file_id":"file-1","account_id":"forged"}`, `{"file_id":"file-1"}{}`, `{`} {
		if result := tool.Handler(t.Context(), adapter.ToolCall{Function: adapter.FunctionCall{Arguments: args}}); result.Error == nil || result.ModelResult != nil {
			t.Fatalf("unsafe arguments accepted: %s", args)
		}
	}
	args := adapter.ToolCall{Function: adapter.FunctionCall{Arguments: `{"file_id":"file-1"}`}}
	owner := files.file.CreatedBy
	files.file.CreatedBy = uuid.NewString()
	if result := tool.Handler(t.Context(), args); result.Error == nil || extractor.calls != 0 {
		t.Fatal("foreign owner reached extraction")
	}
	files.file.CreatedBy = owner
	revoked := false
	prepared.RunConfig.AttachmentReadAuthorizer = func(context.Context, *PreparedChat, *dto.UploadFile) error {
		if revoked {
			return errors.New("private authorization diagnostic")
		}
		return nil
	}
	if result := tool.Handler(t.Context(), args); result.Error != nil {
		t.Fatal(result.Error)
	}
	revoked = true
	if result := tool.Handler(t.Context(), args); result.Error == nil || extractor.calls != 1 {
		t.Fatal("application authorization was cached between pages")
	}
}

func TestAttachmentPreviewChecksPublishedProvenanceBeforeParsing(t *testing.T) {
	svc, prepared, _, extractor := attachmentReaderFixture(t)
	prepared.RunConfig.AttachmentReadAuthorizer = func(context.Context, *PreparedChat, *dto.UploadFile) error {
		return errors.New("file belongs to another application")
	}
	if err := svc.extractPreparedAttachments(t.Context(), prepared, nil); !errors.Is(err, ErrPermissionDenied) || extractor.calls != 0 {
		t.Fatal("unauthorized published upload reached preview extraction")
	}
}

func TestAttachmentReaderRequiresConfirmedModelAndServerEntryGrant(t *testing.T) {
	for _, mutate := range []func(*chatRequestParts){
		func(p *chatRequestParts) { p.FunctionCallingKnown = false },
		func(p *chatRequestParts) { p.ModelSupportsFunctionCalling = false },
		func(p *chatRequestParts) { p.FunctionCallingAssumed = true },
		func(p *chatRequestParts) { p.AttachmentReadAllowed = false },
	} {
		svc, prepared, _, _ := attachmentReaderFixture(t)
		prepared.parts.ExecutionMode = executionModeDirectChat
		mutate(prepared.parts)
		registerAttachmentReadFiles(prepared.parts, nil)
		if prepared.toolLoopEnabled() || len(svc.attachmentRuntimeTools(prepared)) != 0 {
			t.Fatal("attachment read enabled without confirmed model capability or server-owned entry authorization")
		}
	}
}

func TestAttachmentReaderAdmitsOnlyCurrentBranchAndRespectsIsolation(t *testing.T) {
	_, prepared, _, _ := attachmentReaderFixture(t)
	deleted := time.Now()
	branch := []*runtimemodel.Message{{Metadata: map[string]interface{}{"files": []map[string]interface{}{{"id": "ancestor"}}}},
		{DeletedAt: &deleted, Metadata: map[string]interface{}{"files": []map[string]interface{}{{"id": "deleted"}}}}}
	registerAttachmentReadFiles(prepared.parts, branch)
	if len(prepared.parts.AttachmentReadFiles) != 2 || prepared.parts.AttachmentReadFiles["ancestor"].ID == "" {
		t.Fatal("branch membership not respected")
	}
	if len(prepared.RuntimeAttachmentRefs()) != 1 {
		t.Fatal("historical references changed Shell's current-turn staging")
	}
	prepared.parts.HistoryPolicy = &runtimedto.ConversationHistoryPolicy{Mode: runtimedto.ConversationHistoryModeIsolated}
	registerAttachmentReadFiles(prepared.parts, branch)
	if len(prepared.parts.AttachmentReadFiles) != 1 {
		t.Fatal("isolated turn admitted historical attachments")
	}
}

func TestAttachmentReaderContentIsEphemeralInDurableAndLegacyTranscript(t *testing.T) {
	svc, prepared, _, _ := attachmentReaderFixture(t)
	result := svc.attachmentRuntimeTools(prepared)[0].Handler(t.Context(), adapter.ToolCall{Function: adapter.FunctionCall{Arguments: `{"file_id":"file-1","limit":10}`}})
	encoded, _ := json.Marshal(result.ModelResult)
	payload := map[string]interface{}{"role": "tool", "tool_call_id": "read-1", "content": string(encoded)}
	pruned := pruneTranscriptMessageForWrite(uuid.New(), payload, runtimemodel.TranscriptEntryKindTool)
	if strings.Contains(pruned["content"].(string), "汉") || !strings.Contains(pruned["content"].(string), "next_offset") || !strings.Contains(payload["content"].(string), "汉") {
		t.Fatal("durable projection leaked body or mutated the current model result")
	}
	transcript := normalizeAgentTranscript([]adapter.Message{
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "read-1"}}},
		{Role: "tool", ToolCallID: "read-1", Content: string(encoded)},
	}, "")
	if len(transcript) != 2 || strings.Contains(transcript[1].Content.(string), "汉") {
		t.Fatal("legacy transcript retained raw attachment page")
	}
	encodedMessage, err := protocolMessageToJSON(adapter.Message{Role: "tool", ToolCallID: "read-1", Content: string(encoded)})
	if err != nil || strings.Contains(encodedMessage["content"].(string), "汉") {
		t.Fatal("durable full payload retained raw attachment page")
	}
	dbSvc, message, conversation := newContextEntryTestService(t, "attachment-page-durable")
	dbPrepared := &PreparedChat{Message: message, Conversation: conversation, Scope: Scope{OrganizationID: conversation.OrganizationID, AccountID: conversation.AccountID}}
	dbSvc.persistTurnTranscriptEntries(t.Context(), dbPrepared, []adapter.Message{
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "read-1"}}},
		{Role: "tool", ToolCallID: "read-1", Content: string(encoded)},
	})
	var entries []runtimemodel.TranscriptEntry
	if err := dbSvc.repos.DB.Where("message_id = ?", message.ID).Find(&entries).Error; err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		stored, _ := json.Marshal(entry)
		if strings.Contains(string(stored), "汉") {
			t.Fatal("raw page persisted in the transcript database")
		}
	}
}
