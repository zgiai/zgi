package aichat_test

import (
	"archive/zip"
	"bytes"
	"context"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	aichatdto "github.com/zgiai/ginext/internal/modules/aichat/dto"
	"github.com/zgiai/ginext/internal/modules/aichat/repository"
	aichatservice "github.com/zgiai/ginext/internal/modules/aichat/service"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/ginext/internal/modules/skills"
)

func TestService_ImportCustomSkill_AddsDisabledCustomSkill(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join("storage", "aichat", "skills", orgID.String())) })
	seedMember(t, db, orgID, accountID)
	svc := aichatservice.NewServiceWithSkillRuntime(repository.NewRepositories(db), &fakeLLMClient{}, nil, nil, nil, nil, nil, newTestSkillRuntime())
	header := customSkillZipFileHeader(t, map[string]string{
		"SKILL.md":               customSkillMarkdown("brief-writer"),
		"style.md":               "root reference",
		"references/details.md":  "nested reference",
		"assets/template.txt":    "asset",
		"scripts/unsupported.py": "print('not executed')",
	})

	metadata, err := svc.ImportCustomSkill(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, header)
	if err != nil {
		t.Fatalf("ImportCustomSkill() error = %v", err)
	}
	if metadata.ID != "brief-writer" || metadata.Source != skills.SkillSourceCustom || metadata.Enabled {
		t.Fatalf("imported metadata = %#v", metadata)
	}
	if metadata.RuntimeType != skills.SkillRuntimeTypePrompt || metadata.HasTools {
		t.Fatalf("runtime metadata = %#v", metadata)
	}
	if !metadata.HasReferences || !metadata.HasScripts || metadata.ScriptsSupported {
		t.Fatalf("capability metadata = %#v", metadata)
	}

	items, err := svc.ListSkills(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID})
	if err != nil {
		t.Fatalf("ListSkills() error = %v", err)
	}
	if !skillListContains(items, "brief-writer", skills.SkillSourceCustom, false) {
		t.Fatalf("skills = %#v, want disabled custom skill", items)
	}
	config, err := svc.UpdateSkillConfig(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.UpdateSkillConfigRequest{
		EnabledSkillIDs: []string{"brief-writer"},
	})
	if err != nil {
		t.Fatalf("UpdateSkillConfig() error = %v", err)
	}
	if !sameStrings(config.EnabledSkillIDs, []string{"brief-writer"}) {
		t.Fatalf("enabled_skill_ids = %v, want brief-writer", config.EnabledSkillIDs)
	}
}

func TestService_DeleteCustomSkillClearsOrganizationSkillConfig(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join("storage", "aichat", "skills", orgID.String())) })
	seedMember(t, db, orgID, accountID)
	svc := aichatservice.NewServiceWithSkillRuntime(repository.NewRepositories(db), &fakeLLMClient{}, nil, nil, nil, nil, nil, newTestSkillRuntime())
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	header := customSkillZipFileHeader(t, map[string]string{
		"SKILL.md": customSkillMarkdown("brief-writer"),
	})
	if _, err := svc.ImportCustomSkill(context.Background(), scope, header); err != nil {
		t.Fatalf("ImportCustomSkill() error = %v", err)
	}
	if _, err := svc.UpdateSkillConfig(context.Background(), scope, aichatdto.UpdateSkillConfigRequest{EnabledSkillIDs: []string{"brief-writer"}}); err != nil {
		t.Fatalf("UpdateSkillConfig() error = %v", err)
	}
	if err := svc.DeleteSkill(context.Background(), scope, "brief-writer"); err != nil {
		t.Fatalf("DeleteSkill() error = %v", err)
	}

	reupload := customSkillZipFileHeader(t, map[string]string{
		"SKILL.md": customSkillMarkdown("brief-writer"),
	})
	metadata, err := svc.ImportCustomSkill(context.Background(), scope, reupload)
	if err != nil {
		t.Fatalf("ImportCustomSkill() after delete error = %v", err)
	}
	if metadata.Enabled {
		t.Fatalf("reuploaded metadata enabled = true, want false")
	}
	items, err := svc.ListSkills(context.Background(), scope)
	if err != nil {
		t.Fatalf("ListSkills() error = %v", err)
	}
	if !skillListContains(items, "brief-writer", skills.SkillSourceCustom, false) {
		t.Fatalf("skills = %#v, want reuploaded disabled custom skill", items)
	}
}

func TestService_DeleteSystemSkillDoesNotClearOrganizationSkillConfig(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)
	seedOrganizationSkillConfigs(t, db, orgID, map[string]bool{"time": true})
	svc := aichatservice.NewServiceWithSkillRuntime(repository.NewRepositories(db), &fakeLLMClient{}, nil, nil, nil, nil, nil, newTestSkillRuntime())
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}

	err := svc.DeleteSkill(context.Background(), scope, "time")
	if err == nil || !strings.Contains(err.Error(), "system skill cannot be deleted") {
		t.Fatalf("DeleteSkill() error = %v, want system skill deletion error", err)
	}
	config, err := svc.GetSkillConfig(context.Background(), scope)
	if err != nil {
		t.Fatalf("GetSkillConfig() error = %v", err)
	}
	if !sameStrings(config.EnabledSkillIDs, []string{"time"}) {
		t.Fatalf("enabled_skill_ids = %v, want time", config.EnabledSkillIDs)
	}
}

func TestService_ImportCustomSkill_RejectsToolDeclaration(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join("storage", "aichat", "skills", orgID.String())) })
	seedMember(t, db, orgID, accountID)
	svc := aichatservice.NewServiceWithSkillRuntime(repository.NewRepositories(db), &fakeLLMClient{}, nil, nil, nil, nil, nil, newTestSkillRuntime())
	header := customSkillZipFileHeader(t, map[string]string{
		"SKILL.md": `---
name: bad-skill
description: Invalid tool custom skill.
tools:
  - calculate
---

# Bad Skill
`,
	})

	_, err := svc.ImportCustomSkill(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, header)
	if err == nil || !strings.Contains(err.Error(), "must use prompt runtime_type") {
		t.Fatalf("ImportCustomSkill() error = %v, want prompt-only validation", err)
	}
	if _, statErr := os.Stat(filepath.Join("storage", "aichat", "skills", orgID.String(), "bad-skill", "current")); !os.IsNotExist(statErr) {
		t.Fatalf("custom skill directory stat error = %v, want not exists", statErr)
	}
}

func TestService_RunPreparedStreamWithCustomPromptSkillLoadsInstructions(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join("storage", "aichat", "skills", orgID.String())) })
	seedMember(t, db, orgID, accountID)
	fakeLLM := &fakeLLMClient{
		chunks: []string{"Draft complete."},
		appChatResponses: []*adapter.ChatResponse{
			toolCallPlanningResponse("load_skill", `{"skill_id":"brief-writer"}`),
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "Ready"}}}},
		},
	}
	svc := aichatservice.NewServiceWithSkillRuntime(repository.NewRepositories(db), fakeLLM, nil, functionCallingModelResolver(), nil, nil, nil, newTestSkillRuntime())
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	header := customSkillZipFileHeader(t, map[string]string{
		"SKILL.md": customSkillMarkdown("brief-writer"),
		"style.md": "root reference",
	})
	if _, err := svc.ImportCustomSkill(context.Background(), scope, header); err != nil {
		t.Fatalf("ImportCustomSkill() error = %v", err)
	}
	if _, err := svc.UpdateSkillConfig(context.Background(), scope, aichatdto.UpdateSkillConfigRequest{EnabledSkillIDs: []string{"brief-writer"}}); err != nil {
		t.Fatalf("UpdateSkillConfig() error = %v", err)
	}
	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{Query: "Draft a brief.", Model: "gpt-test"})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	result, err := svc.RunPreparedStream(context.Background(), prepared, nil)
	if err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	if result.Answer != "Draft complete." {
		t.Fatalf("answer = %q, want streamed answer", result.Answer)
	}
	if len(fakeLLM.appChatRequests) != 2 {
		t.Fatalf("planning request count = %d, want 2", len(fakeLLM.appChatRequests))
	}
	if len(fakeLLM.appChatRequests[0].Tools) != 2 || containsTool(fakeLLM.appChatRequests[0].Tools, skills.MetaToolCallSkillTool) {
		t.Fatalf("planning tools = %#v, want load/read only", fakeLLM.appChatRequests[0].Tools)
	}
	if !requestMessagesContain(fakeLLM.appChatRequests[1], "Brief Writer") {
		t.Fatalf("second planning request did not contain loaded skill instructions")
	}
}

func customSkillZipFileHeader(t *testing.T, files map[string]string) *multipart.FileHeader {
	t.Helper()
	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)
	for name, content := range files {
		writer, err := zipWriter.Create(name)
		if err != nil {
			t.Fatalf("create zip file: %v", err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatalf("write zip file: %v", err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	var body bytes.Buffer
	formWriter := multipart.NewWriter(&body)
	part, err := formWriter.CreateFormFile("file", "skill.zip")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(zipBuffer.Bytes()); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := formWriter.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req := httptest.NewRequest("POST", "/skills/import", &body)
	req.Header.Set("Content-Type", formWriter.FormDataContentType())
	if err := req.ParseMultipartForm(32 << 20); err != nil {
		t.Fatalf("parse multipart: %v", err)
	}
	headers := req.MultipartForm.File["file"]
	if len(headers) != 1 {
		t.Fatalf("file header count = %d, want 1", len(headers))
	}
	return headers[0]
}

func customSkillMarkdown(skillID string) string {
	return `---
name: ` + skillID + `
description: Help draft short writing briefs.
display:
  icon: file-text
  label:
    en_US: Brief Writer
---

# Brief Writer

Use the references before drafting a brief.
`
}

func skillListContains(items []skills.SkillDiscoveryMetadata, id string, source string, enabled bool) bool {
	for _, item := range items {
		if item.ID == id && item.Source == source && item.Enabled == enabled {
			return true
		}
	}
	return false
}
