package aichat_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	appconfig "github.com/zgiai/ginext/config"
	aichatdto "github.com/zgiai/ginext/internal/modules/aichat/dto"
	aichatmodel "github.com/zgiai/ginext/internal/modules/aichat/model"
	"github.com/zgiai/ginext/internal/modules/aichat/repository"
	aichatservice "github.com/zgiai/ginext/internal/modules/aichat/service"
	"github.com/zgiai/ginext/internal/modules/app/workflow/tool_file"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/ginext/internal/modules/skills"
	"github.com/zgiai/ginext/internal/modules/tools"
	calculatorpkg "github.com/zgiai/ginext/internal/modules/tools/builtin/calculator"
	filegeneratorpkg "github.com/zgiai/ginext/internal/modules/tools/builtin/filegenerator"
	timepkg "github.com/zgiai/ginext/internal/modules/tools/builtin/time"
	"github.com/zgiai/ginext/pkg/storage"
	"gorm.io/gorm"
)

func TestService_RunPreparedStreamWithTimeSkillExecutesToolCall(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	fakeLLM := &fakeLLMClient{
		chunks: []string{"It is ", "today."},
		appChatResponses: []*adapter.ChatResponse{
			toolCallPlanningResponse("load_skill", `{"skill_id":"time"}`),
			toolCallPlanningResponse("call_skill_tool", `{"skill_id":"time","tool_name":"current_time","arguments":{"timezone":"UTC","format":"%Y-%m-%d"}}`),
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "Ready to answer"}}}},
		},
	}
	svc := newFunctionCallingSkillService(db, fakeLLM, newTestSkillRuntime())
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{
		Query: "What date is it in UTC?",
		Model: "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	var events []aichatservice.StreamEvent
	result, err := svc.RunPreparedStream(context.Background(), prepared, nil, func(event aichatservice.StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	if result.Answer != "It is today." {
		t.Fatalf("answer = %q, want final streamed answer", result.Answer)
	}
	if len(fakeLLM.appChatRequests) != 3 {
		t.Fatalf("planning request count = %d, want 3", len(fakeLLM.appChatRequests))
	}
	if len(fakeLLM.appChatRequests[0].Tools) != 3 {
		t.Fatalf("planning tools = %d, want 3 meta tools", len(fakeLLM.appChatRequests[0].Tools))
	}
	if containsTool(fakeLLM.appChatRequests[0].Tools, "current_time") || containsTool(fakeLLM.appChatRequests[0].Tools, "date_calculate") {
		t.Fatalf("planning tools include business tools: %#v", fakeLLM.appChatRequests[0].Tools)
	}
	if requestMessagesContain(fakeLLM.appChatRequests[0], "current_time") || requestMessagesContain(fakeLLM.appChatRequests[0], "date_calculate") {
		t.Fatalf("planning metadata includes business tool names: %#v", fakeLLM.appChatRequests[0].Messages)
	}
	if !requestContainsToolResult(fakeLLM.appChatRequests[1], "call_1") {
		t.Fatalf("second planning request does not include loaded skill result")
	}
	if len(fakeLLM.requests) != 1 {
		t.Fatalf("stream request count = %d, want 1", len(fakeLLM.requests))
	}
	finalMessages := fakeLLM.requests[0].Messages
	if len(finalMessages) < 4 {
		t.Fatalf("final message count = %d, want tool messages included", len(finalMessages))
	}
	if finalMessages[len(finalMessages)-1].Role != "tool" || finalMessages[len(finalMessages)-1].ToolCallID == "" {
		t.Fatalf("last final message = %#v, want tool result", finalMessages[len(finalMessages)-1])
	}
	types := eventTypes(events)
	if !containsString(types, "skill_load_start") || !containsString(types, "skill_load_end") {
		t.Fatalf("events = %v, want skill load start and end", types)
	}
	if !containsString(types, "skill_call_start") || !containsString(types, "skill_call_end") {
		t.Fatalf("events = %v, want skill call start and end", types)
	}
	if result.Metadata["has_trace"] != true {
		t.Fatalf("has_trace = %v, want true", result.Metadata["has_trace"])
	}
	if metadataInt(result.Metadata["skill_call_count"]) != 2 {
		t.Fatalf("skill_call_count = %v, want 2", result.Metadata["skill_call_count"])
	}
	if metadataInt(result.Metadata["tool_call_count"]) != 1 {
		t.Fatalf("tool_call_count = %v, want 1", result.Metadata["tool_call_count"])
	}
}

func TestService_RunPreparedStreamWithCalculatorSkillExecutesToolCall(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	fakeLLM := &fakeLLMClient{
		chunks: []string{"12 times 3 is 36."},
		appChatResponses: []*adapter.ChatResponse{
			toolCallPlanningResponse("load_skill", `{"skill_id":"calculator"}`),
			toolCallPlanningResponse("call_skill_tool", `{"skill_id":"calculator","tool_name":"calculate","arguments":{"operation":"multiply","left":12,"right":3,"precision":2}}`),
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "Ready to answer"}}}},
		},
	}
	svc := newFunctionCallingSkillService(db, fakeLLM, newTestSkillRuntime())
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{
		Query: "What is 12 times 3?",
		Model: "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	var events []aichatservice.StreamEvent
	result, err := svc.RunPreparedStream(context.Background(), prepared, nil, func(event aichatservice.StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	if result.Answer != "12 times 3 is 36." {
		t.Fatalf("answer = %q, want final streamed answer", result.Answer)
	}
	if len(fakeLLM.appChatRequests) != 3 {
		t.Fatalf("planning request count = %d, want 3", len(fakeLLM.appChatRequests))
	}
	if containsTool(fakeLLM.appChatRequests[0].Tools, "calculate") || containsTool(fakeLLM.appChatRequests[0].Tools, "percentage") {
		t.Fatalf("planning tools include business tools: %#v", fakeLLM.appChatRequests[0].Tools)
	}
	if requestMessagesContain(fakeLLM.appChatRequests[0], "calculate") || requestMessagesContain(fakeLLM.appChatRequests[0], "percentage") {
		t.Fatalf("planning metadata includes business tool names: %#v", fakeLLM.appChatRequests[0].Messages)
	}
	if !requestContainsToolResult(fakeLLM.appChatRequests[1], "call_1") {
		t.Fatalf("second planning request does not include loaded skill result")
	}
	types := eventTypes(events)
	if !containsString(types, "skill_load_start") || !containsString(types, "skill_load_end") {
		t.Fatalf("events = %v, want skill load start and end", types)
	}
	if !containsString(types, "skill_call_start") || !containsString(types, "skill_call_end") {
		t.Fatalf("events = %v, want skill call start and end", types)
	}
	if metadataInt(result.Metadata["tool_call_count"]) != 1 {
		t.Fatalf("tool_call_count = %v, want 1", result.Metadata["tool_call_count"])
	}
}

func TestService_RunPreparedStreamWithCalculatorSkillAllowsMultiStepArithmetic(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	fakeLLM := &fakeLLMClient{
		chunks: []string{"Final amount is 180.20."},
		appChatResponses: []*adapter.ChatResponse{
			toolCallPlanningResponse("load_skill", `{"skill_id":"calculator"}`),
			toolCallPlanningResponse("call_skill_tool", `{"skill_id":"calculator","tool_name":"calculate","arguments":{"operation":"multiply","left":9.5,"right":10}}`),
			toolCallPlanningResponse("call_skill_tool", `{"skill_id":"calculator","tool_name":"calculate","arguments":{"operation":"multiply","left":4.8,"right":15}}`),
			toolCallPlanningResponse("call_skill_tool", `{"skill_id":"calculator","tool_name":"calculate","arguments":{"operation":"multiply","left":15,"right":5}}`),
			toolCallPlanningResponse("call_skill_tool", `{"skill_id":"calculator","tool_name":"calculate","arguments":{"operation":"add","left":95,"right":72}}`),
			toolCallPlanningResponse("call_skill_tool", `{"skill_id":"calculator","tool_name":"calculate","arguments":{"operation":"add","left":167,"right":75}}`),
			toolCallPlanningResponse("call_skill_tool", `{"skill_id":"calculator","tool_name":"calculate","arguments":{"operation":"subtract","left":242,"right":30}}`),
			toolCallPlanningResponse("call_skill_tool", `{"skill_id":"calculator","tool_name":"percentage","arguments":{"operation":"apply_decrease","value":212,"percent":15,"precision":2}}`),
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "Ready to answer"}}}},
		},
	}
	svc := newFunctionCallingSkillService(db, fakeLLM, newTestSkillRuntime())
	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query: "Calculate a multi-step grocery discount.",
		Model: "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	result, err := svc.RunPreparedStream(context.Background(), prepared, nil)
	if err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	if result.Answer != "Final amount is 180.20." {
		t.Fatalf("answer = %q, want final streamed answer", result.Answer)
	}
	if metadataInt(result.Metadata["tool_call_count"]) != 7 {
		t.Fatalf("tool_call_count = %v, want 7", result.Metadata["tool_call_count"])
	}
}

func TestService_RunPreparedStreamWithCalculatorExpressionReducesToolCalls(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	fakeLLM := &fakeLLMClient{
		chunks: []string{"Final amount is 180.20."},
		appChatResponses: []*adapter.ChatResponse{
			toolCallPlanningResponse("load_skill", `{"skill_id":"calculator"}`),
			toolCallPlanningResponse("call_skill_tool", `{"skill_id":"calculator","tool_name":"evaluate_expression","arguments":{"expression":"(9.5*10 + 4.8*15 + 15*5 - 30) * 0.85","precision":2}}`),
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "Ready to answer"}}}},
		},
	}
	svc := newFunctionCallingSkillService(db, fakeLLM, newTestSkillRuntime())
	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query: "Calculate a multi-step grocery discount.",
		Model: "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	result, err := svc.RunPreparedStream(context.Background(), prepared, nil)
	if err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	if result.Answer != "Final amount is 180.20." {
		t.Fatalf("answer = %q, want final streamed answer", result.Answer)
	}
	if metadataInt(result.Metadata["tool_call_count"]) != 1 {
		t.Fatalf("tool_call_count = %v, want 1", result.Metadata["tool_call_count"])
	}
	if !requestContainsToolResult(fakeLLM.appChatRequests[2], "call_1") {
		t.Fatalf("final planning request does not include expression tool result")
	}
}

func TestService_RunPreparedStreamWithFileGeneratorEmitsArtifactEvent(t *testing.T) {
	setupAIChatToolFileGlobals(t)
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	fakeLLM := &fakeLLMClient{
		chunks: []string{"File generated."},
		appChatResponses: []*adapter.ChatResponse{
			toolCallPlanningResponse("load_skill", `{"skill_id":"file-generator"}`),
			toolCallPlanningResponse("call_skill_tool", `{"skill_id":"file-generator","tool_name":"generate_file","arguments":{"content":"# Notes\nGenerated content","format":"md","filename":"notes"}}`),
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "Ready to answer"}}}},
		},
	}
	svc := newFunctionCallingSkillService(db, fakeLLM, newTestSkillRuntime())
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{
		Query: "Create a markdown file.",
		Model: "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	var events []aichatservice.StreamEvent
	result, err := svc.RunPreparedStream(context.Background(), prepared, nil, func(event aichatservice.StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	artifactEvent := firstEventPayload(events, "skill_artifact_created")
	if artifactEvent == nil {
		t.Fatalf("events = %v, want skill_artifact_created", eventTypes(events))
	}
	if metadataString(artifactEvent["artifact_type"]) != "file" || metadataString(artifactEvent["skill_id"]) != "file-generator" {
		t.Fatalf("artifact event = %#v", artifactEvent)
	}
	if metadataString(artifactEvent["filename"]) != "notes.md" {
		t.Fatalf("filename = %v, want notes.md", artifactEvent["filename"])
	}
	if metadataString(artifactEvent["url"]) == "" || metadataString(artifactEvent["file_id"]) == "" {
		t.Fatalf("artifact event missing url or file_id: %#v", artifactEvent)
	}
	if metadataString(artifactEvent["download_url"]) == "" {
		t.Fatalf("artifact event missing download_url: %#v", artifactEvent)
	}
	if strings.Contains(metadataString(artifactEvent["url"]), "download=1") {
		t.Fatalf("artifact preview url contains download flag: %#v", artifactEvent)
	}
	if !strings.Contains(metadataString(artifactEvent["download_url"]), "download=1") {
		t.Fatalf("artifact download_url missing download flag: %#v", artifactEvent)
	}
	if generatedFilesCount(result.Metadata["generated_files"]) != 1 || metadataInt(result.Metadata["generated_file_count"]) != 1 {
		t.Fatalf("generated files metadata = %#v", result.Metadata)
	}
	resultFile := firstGeneratedFile(result.Metadata["generated_files"])
	if metadataString(resultFile["url"]) != "" || metadataString(resultFile["download_url"]) != "" {
		t.Fatalf("result generated_files persisted signed urls: %#v", resultFile)
	}

	var stored aichatmodel.Message
	if err := db.Where("id = ?", prepared.Message.ID).Take(&stored).Error; err != nil {
		t.Fatalf("load message: %v", err)
	}
	if generatedFilesCount(stored.Metadata["generated_files"]) != 1 {
		t.Fatalf("stored generated_files = %#v, want one file", stored.Metadata["generated_files"])
	}
	storedFile := firstGeneratedFile(stored.Metadata["generated_files"])
	if metadataString(storedFile["url"]) != "" || metadataString(storedFile["download_url"]) != "" {
		t.Fatalf("stored generated_files persisted signed urls: %#v", storedFile)
	}

	messages, _, err := svc.ListMessages(context.Background(), scope, prepared.Conversation.ID, 1, 20)
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	hydratedFile := firstGeneratedFile(findMessageMetadata(messages, prepared.Message.ID)["generated_files"])
	if metadataString(hydratedFile["url"]) == "" || metadataString(hydratedFile["download_url"]) == "" {
		t.Fatalf("hydrated generated_files missing urls: %#v", hydratedFile)
	}
	if strings.Contains(metadataString(hydratedFile["url"]), "download=1") {
		t.Fatalf("hydrated preview url contains download flag: %#v", hydratedFile)
	}
	if !strings.Contains(metadataString(hydratedFile["download_url"]), "download=1") {
		t.Fatalf("hydrated download_url missing download flag: %#v", hydratedFile)
	}

	callStart := firstEventPayload(events, "skill_call_start")
	if callStart == nil {
		t.Fatalf("events = %v, want skill_call_start", eventTypes(events))
	}
	if _, ok := callStart["arguments"]; ok {
		t.Fatalf("skill_call_start contains raw arguments: %#v", callStart)
	}
	summary, _ := callStart["arguments_summary"].(map[string]interface{})
	if metadataString(summary["content"]) != "" || metadataInt(summary["content_length"]) == 0 {
		t.Fatalf("arguments_summary = %#v, want content_length without content", summary)
	}
	invocationArgs := firstInvocationArguments(stored.Metadata["skill_invocations"], "file-generator", "generate_file")
	if metadataString(invocationArgs["content"]) != "" || metadataInt(invocationArgs["content_length"]) == 0 {
		t.Fatalf("skill invocation arguments = %#v, want content_length without content", invocationArgs)
	}
}

func TestService_StreamConversationEventsResignsArtifactEventURLs(t *testing.T) {
	setupAIChatToolFileGlobals(t)
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)
	withRedisClient(t)

	fakeLLM := &fakeLLMClient{
		chunks: []string{"File generated."},
		appChatResponses: []*adapter.ChatResponse{
			toolCallPlanningResponse("load_skill", `{"skill_id":"file-generator"}`),
			toolCallPlanningResponse("call_skill_tool", `{"skill_id":"file-generator","tool_name":"generate_file","arguments":{"content":"replay content","format":"md","filename":"replay"}}`),
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "Ready to answer"}}}},
		},
	}
	svc := newFunctionCallingSkillService(db, fakeLLM, newTestSkillRuntime())
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{
		Query: "Create a replay markdown file.",
		Model: "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	var liveEvents []aichatservice.StreamEvent
	if _, err := svc.RunPreparedStream(context.Background(), prepared, nil, func(event aichatservice.StreamEvent) error {
		liveEvents = append(liveEvents, event)
		return nil
	}); err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	liveArtifact := firstEventPayload(liveEvents, "skill_artifact_created")
	if liveArtifact == nil || metadataString(liveArtifact["url"]) == "" {
		t.Fatalf("live events = %v, want artifact url", eventTypes(liveEvents))
	}

	var replayEvents []aichatservice.StreamEvent
	if err := svc.StreamConversationEvents(context.Background(), scope, prepared.Conversation.ID, prepared.Message.ID, "0", func(event aichatservice.StreamEvent) error {
		replayEvents = append(replayEvents, event)
		return nil
	}); err != nil {
		t.Fatalf("StreamConversationEvents() error = %v", err)
	}
	replayArtifact := firstEventPayload(replayEvents, "skill_artifact_created")
	if replayArtifact == nil {
		t.Fatalf("replay events = %v, want skill_artifact_created", eventTypes(replayEvents))
	}
	if metadataString(replayArtifact["url"]) == "" || metadataString(replayArtifact["download_url"]) == "" {
		t.Fatalf("replay artifact missing urls: %#v", replayArtifact)
	}
	if metadataString(replayArtifact["url"]) == metadataString(liveArtifact["url"]) {
		t.Fatalf("replay artifact url was not re-signed: live=%s replay=%s", liveArtifact["url"], replayArtifact["url"])
	}
	if strings.Contains(metadataString(replayArtifact["url"]), "download=1") {
		t.Fatalf("replay preview url contains download flag: %#v", replayArtifact)
	}
	if !strings.Contains(metadataString(replayArtifact["download_url"]), "download=1") {
		t.Fatalf("replay download_url missing download flag: %#v", replayArtifact)
	}
}

func TestService_RunPreparedStreamEnforcesPerSkillToolCallLimit(t *testing.T) {
	setupAIChatToolFileGlobals(t)
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	fakeLLM := &fakeLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			toolCallPlanningResponse("load_skill", `{"skill_id":"file-generator"}`),
			toolCallPlanningResponse("call_skill_tool", `{"skill_id":"file-generator","tool_name":"generate_file","arguments":{"content":"first file","format":"txt","filename":"one"}}`),
			toolCallPlanningResponse("call_skill_tool", `{"skill_id":"file-generator","tool_name":"generate_file","arguments":{"content":"second file","format":"txt","filename":"two"}}`),
			toolCallPlanningResponse("call_skill_tool", `{"skill_id":"file-generator","tool_name":"generate_file","arguments":{"content":"third file","format":"txt","filename":"three"}}`),
			toolCallPlanningResponse("call_skill_tool", `{"skill_id":"file-generator","tool_name":"generate_file","arguments":{"content":"fourth file should not execute","format":"txt","filename":"four"}}`),
		},
	}
	svc := newFunctionCallingSkillService(db, fakeLLM, newTestSkillRuntime())
	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query: "Create four text files.",
		Model: "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	var events []aichatservice.StreamEvent
	_, err = svc.RunPreparedStream(context.Background(), prepared, nil, func(event aichatservice.StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if !errors.Is(err, aichatservice.ErrInvalidInput) || !strings.Contains(err.Error(), "too many skill tool calls for skill file-generator") {
		t.Fatalf("RunPreparedStream() error = %v, want file-generator skill limit", err)
	}
	if got := eventCount(events, "skill_call_start"); got != 3 {
		t.Fatalf("skill_call_start events = %d, want 3", got)
	}
	if got := eventCount(events, "skill_artifact_created"); got != 3 {
		t.Fatalf("skill_artifact_created events = %d, want 3", got)
	}
	if got := eventCount(events, "skill_call_error"); got != 1 {
		t.Fatalf("skill_call_error events = %d, want 1", got)
	}

	var stored aichatmodel.Message
	if loadErr := db.Where("id = ?", prepared.Message.ID).Take(&stored).Error; loadErr != nil {
		t.Fatalf("load message: %v", loadErr)
	}
	if generatedFilesCount(stored.Metadata["generated_files"]) != 3 {
		t.Fatalf("stored generated_files = %#v, want three files", stored.Metadata["generated_files"])
	}
	limitTrace := lastInvocation(stored.Metadata["skill_invocations"], "file-generator", "generate_file")
	if metadataString(limitTrace["status"]) != "error" {
		t.Fatalf("last invocation = %#v, want error trace", limitTrace)
	}
	if !strings.Contains(metadataString(limitTrace["error"]), "too many skill tool calls for skill file-generator") {
		t.Fatalf("last invocation error = %#v, want skill limit error", limitTrace)
	}
	args, _ := limitTrace["arguments"].(map[string]interface{})
	if metadataString(args["content"]) != "" || metadataInt(args["content_length"]) == 0 {
		t.Fatalf("limit trace arguments = %#v, want summarized content length only", args)
	}
	if metadataInt(stored.Metadata["tool_call_count"]) != 4 {
		t.Fatalf("tool_call_count = %v, want 4 including failed limit trace", stored.Metadata["tool_call_count"])
	}
}

func TestService_RunPreparedStreamEmitsSkillErrorWhenCalculatorFails(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	fakeLLM := &fakeLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			toolCallPlanningResponse("load_skill", `{"skill_id":"calculator"}`),
			toolCallPlanningResponse("call_skill_tool", `{"skill_id":"calculator","tool_name":"calculate","arguments":{"operation":"divide","left":12,"right":0}}`),
		},
	}
	svc := newFunctionCallingSkillService(db, fakeLLM, newTestSkillRuntime())
	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query: "What is 12 divided by 0?",
		Model: "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	var events []aichatservice.StreamEvent
	_, err = svc.RunPreparedStream(context.Background(), prepared, nil, func(event aichatservice.StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err == nil {
		t.Fatalf("RunPreparedStream() error = nil, want tool error")
	}
	types := eventTypes(events)
	if !containsString(types, "skill_call_error") {
		t.Fatalf("events = %v, want skill_call_error", types)
	}
	var stored aichatmodel.Message
	if loadErr := db.Where("id = ?", prepared.Message.ID).Take(&stored).Error; loadErr != nil {
		t.Fatalf("load message: %v", loadErr)
	}
	if stored.Status != aichatmodel.MessageStatusError {
		t.Fatalf("message status = %q, want error", stored.Status)
	}
}

func TestService_RunPreparedStreamCompletesWhenDefaultAutoModeDoesNotCallTool(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	fakeLLM := &fakeLLMClient{
		chunks: []string{"No tool needed."},
		appChatResponses: []*adapter.ChatResponse{
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "No tool needed"}}}},
		},
	}
	svc := newFunctionCallingSkillService(db, fakeLLM, newTestSkillRuntime())
	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query: "Use a tool.",
		Model: "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	result, err := svc.RunPreparedStream(context.Background(), prepared, nil)
	if err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	if result.Answer != "No tool needed." {
		t.Fatalf("answer = %q, want final streamed answer", result.Answer)
	}
	var stored aichatmodel.Message
	if loadErr := db.Where("id = ?", prepared.Message.ID).Take(&stored).Error; loadErr != nil {
		t.Fatalf("load message: %v", loadErr)
	}
	if stored.Status != aichatmodel.MessageStatusCompleted {
		t.Fatalf("message status = %q, want completed", stored.Status)
	}
}

func TestService_RunPreparedStreamEmitsSkillErrorWhenToolFails(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	fakeLLM := &fakeLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			toolCallPlanningResponse("load_skill", `{"skill_id":"time"}`),
			toolCallPlanningResponse("call_skill_tool", `{"skill_id":"time","tool_name":"date_calculate","arguments":{"operation":"add","base_date":"2026-05-16","amount":1,"unit":"century"}}`),
		},
	}
	svc := newFunctionCallingSkillService(db, fakeLLM, newTestSkillRuntime())
	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query: "Calculate a date.",
		Model: "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	var events []aichatservice.StreamEvent
	_, err = svc.RunPreparedStream(context.Background(), prepared, nil, func(event aichatservice.StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err == nil {
		t.Fatalf("RunPreparedStream() error = nil, want tool error")
	}
	types := eventTypes(events)
	if !containsString(types, "skill_call_error") {
		t.Fatalf("events = %v, want skill_call_error", types)
	}
	var stored aichatmodel.Message
	if loadErr := db.Where("id = ?", prepared.Message.ID).Take(&stored).Error; loadErr != nil {
		t.Fatalf("load message: %v", loadErr)
	}
	if stored.Metadata["has_trace"] != true {
		t.Fatalf("stored has_trace = %v, want true", stored.Metadata["has_trace"])
	}
}

func TestService_RunPreparedStreamRecoversWhenToolCallHappensBeforeSkillLoad(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	fakeLLM := &fakeLLMClient{
		chunks: []string{"The time is available."},
		appChatResponses: []*adapter.ChatResponse{
			toolCallPlanningResponse("call_skill_tool", `{"skill_id":"time","tool_name":"current_time","arguments":{"timezone":"UTC"}}`),
			toolCallPlanningResponse("load_skill", `{"skill_id":"time"}`),
			toolCallPlanningResponse("call_skill_tool", `{"skill_id":"time","tool_name":"current_time","arguments":{"timezone":"UTC"}}`),
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "Ready"}}}},
		},
	}
	svc := newFunctionCallingSkillService(db, fakeLLM, newTestSkillRuntime())
	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query: "What time is it?",
		Model: "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	var events []aichatservice.StreamEvent
	result, err := svc.RunPreparedStream(context.Background(), prepared, nil, func(event aichatservice.StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	if result.Answer != "The time is available." {
		t.Fatalf("answer = %q, want streamed final answer", result.Answer)
	}
	if !requestContainsToolResult(fakeLLM.appChatRequests[1], "call_1") {
		t.Fatalf("second planning request does not include guardrail tool result")
	}
	types := eventTypes(events)
	if containsString(types, "skill_call_error") {
		t.Fatalf("events = %v, did not want skill_call_error for guardrail", types)
	}
	if metadataInt(result.Metadata["guardrail_count"]) != 1 {
		t.Fatalf("guardrail_count = %v, want 1", result.Metadata["guardrail_count"])
	}
	if metadataInt(result.Metadata["tool_call_count"]) != 1 {
		t.Fatalf("tool_call_count = %v, want 1", result.Metadata["tool_call_count"])
	}
}

func TestService_RunPreparedStreamFailsWhenSkillStepLimitExceeded(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	fakeLLM := &fakeLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			manyToolCallPlanningResponse("load_skill", `{"skill_id":"time"}`, 21),
		},
	}
	svc := newFunctionCallingSkillService(db, fakeLLM, newTestSkillRuntime())
	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query: "Use too many skill steps.",
		Model: "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	_, err = svc.RunPreparedStream(context.Background(), prepared, nil)
	if !errors.Is(err, aichatservice.ErrInvalidInput) {
		t.Fatalf("RunPreparedStream() error = %v, want ErrInvalidInput", err)
	}
}

func TestService_RunPreparedStreamFailsWhenModelContinuesAfterSkillStepLimit(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	fakeLLM := &fakeLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			manyToolCallPlanningResponse("load_skill", `{"skill_id":"time"}`, 20),
			toolCallPlanningResponse("load_skill", `{"skill_id":"time"}`),
		},
	}
	svc := newFunctionCallingSkillService(db, fakeLLM, newTestSkillRuntime())
	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query: "Keep calling skills.",
		Model: "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	_, err = svc.RunPreparedStream(context.Background(), prepared, nil)
	if !errors.Is(err, aichatservice.ErrInvalidInput) || !strings.Contains(err.Error(), "too many skill steps") {
		t.Fatalf("RunPreparedStream() error = %v, want too many skill steps", err)
	}
}

func TestService_RunPreparedStreamIgnoresSkillWhenModelDoesNotSupportToolCalling(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	fakeLLM := &fakeLLMClient{chunks: []string{"Plain answer."}}
	svc := aichatservice.NewServiceWithSkillRuntime(
		repository.NewRepositories(db),
		fakeLLM,
		nil,
		&fakeModelSpecResolver{ok: true, spec: aichatservice.ModelSpec{ContextWindow: 4096, SupportsToolCall: false}},
		nil,
		nil,
		nil,
		newTestSkillRuntime(),
	)
	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query: "What time is it?",
		Model: "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	result, err := svc.RunPreparedStream(context.Background(), prepared, nil)
	if err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	if result.Answer != "Plain answer." {
		t.Fatalf("answer = %q, want plain streamed answer", result.Answer)
	}
	if len(fakeLLM.appChatRequests) != 0 {
		t.Fatalf("planning request count = %d, want 0", len(fakeLLM.appChatRequests))
	}
	assertNoSkillRuntimeMetadata(t, prepared.Message.Metadata)
	if _, ok := prepared.Conversation.Metadata["skill_config"]; ok {
		t.Fatalf("conversation metadata = %#v, want no skill_config write", prepared.Conversation.Metadata)
	}
}

func TestService_RunPreparedStreamUsesPlainChatWhenModelDoesNotSupportToolCalling(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	fakeLLM := &fakeLLMClient{chunks: []string{"Required mode also falls back."}}
	svc := aichatservice.NewServiceWithSkillRuntime(
		repository.NewRepositories(db),
		fakeLLM,
		nil,
		&fakeModelSpecResolver{ok: true, spec: aichatservice.ModelSpec{ContextWindow: 4096, SupportsToolCall: false}},
		nil,
		nil,
		nil,
		newTestSkillRuntime(),
	)
	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query: "Use the time skill.",
		Model: "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	result, err := svc.RunPreparedStream(context.Background(), prepared, nil)
	if err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	if result.Answer != "Required mode also falls back." {
		t.Fatalf("answer = %q, want plain streamed answer", result.Answer)
	}
	if len(fakeLLM.appChatRequests) != 0 {
		t.Fatalf("planning request count = %d, want 0", len(fakeLLM.appChatRequests))
	}
	assertNoSkillRuntimeMetadata(t, prepared.Message.Metadata)
}

func TestService_PrepareChatIgnoresPromptSkillWhenModelDoesNotSupportToolCalling(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)
	seedOrganizationSkillConfigs(t, db, orgID, map[string]bool{
		"style-guide": true,
	})

	fakeLLM := &fakeLLMClient{chunks: []string{"Plain prompt skill answer."}}
	svc := aichatservice.NewServiceWithSkillRuntime(
		repository.NewRepositories(db),
		fakeLLM,
		nil,
		&fakeModelSpecResolver{ok: true, spec: aichatservice.ModelSpec{ContextWindow: 4096, SupportsToolCall: false}},
		nil,
		nil,
		nil,
		newPromptSkillRuntime(t),
	)
	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query: "Use the writing style.",
		Model: "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	assertNoSkillRuntimeMetadata(t, prepared.Message.Metadata)
	result, err := svc.RunPreparedStream(context.Background(), prepared, nil)
	if err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	if result.Answer != "Plain prompt skill answer." {
		t.Fatalf("answer = %q, want plain streamed answer", result.Answer)
	}
	if len(fakeLLM.appChatRequests) != 0 {
		t.Fatalf("planning request count = %d, want 0", len(fakeLLM.appChatRequests))
	}
}

func TestService_RunPreparedStreamIgnoresSkillWhenModelMetadataMissing(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	fakeLLM := &fakeLLMClient{
		chunks: []string{"done"},
	}
	svc := aichatservice.NewServiceWithSkillRuntime(
		repository.NewRepositories(db),
		fakeLLM,
		nil,
		&fakeModelSpecResolver{},
		nil,
		nil,
		nil,
		newTestSkillRuntime(),
	)
	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query: "Maybe use time.",
		Model: "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	if _, err := svc.RunPreparedStream(context.Background(), prepared, nil); err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	if len(fakeLLM.appChatRequests) != 0 {
		t.Fatalf("planning request count = %d, want 0", len(fakeLLM.appChatRequests))
	}
	assertNoSkillRuntimeMetadata(t, prepared.Message.Metadata)
}

func newFunctionCallingSkillService(db *gorm.DB, fakeLLM *fakeLLMClient, runtime *skills.Runtime) aichatservice.Service {
	return aichatservice.NewServiceWithSkillRuntime(
		repository.NewRepositories(db),
		fakeLLM,
		nil,
		functionCallingModelResolver(),
		nil,
		nil,
		nil,
		runtime,
	)
}

func assertNoSkillRuntimeMetadata(t *testing.T, metadata map[string]interface{}) {
	t.Helper()
	for _, key := range []string{
		"skill_mode",
		"configured_skill_ids",
		"skill_invocations",
		"has_trace",
		"skill_call_count",
		"skill_names",
		"tool_call_count",
		"tool_names",
		"guardrail_count",
	} {
		if _, ok := metadata[key]; ok {
			t.Fatalf("metadata[%s] = %v, want omitted when skills are disabled", key, metadata[key])
		}
	}
}

func newTestSkillRuntime() *skills.Runtime {
	manager := tools.NewToolManager(nil)
	_ = manager.RegisterProvider(timepkg.NewTimeProvider())
	_ = manager.RegisterProvider(calculatorpkg.NewProvider())
	_ = manager.RegisterProvider(filegeneratorpkg.NewProvider())
	return skills.NewRuntime(tools.NewToolEngine(manager), manager)
}

func newPromptSkillRuntime(t *testing.T) *skills.Runtime {
	t.Helper()
	catalogDir := t.TempDir()
	skillDir := filepath.Join(catalogDir, "style-guide")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir prompt skill: %v", err)
	}
	markdown := `---
name: style-guide
description: Provide concise answer style guidance.
when_to_use: Use when the answer should follow a specific writing style.
runtime_type: prompt
max_calls_per_turn: 1
timeout_seconds: 5
---

# Style Guide Skill

Answer with concise wording.
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(markdown), 0o644); err != nil {
		t.Fatalf("write prompt skill: %v", err)
	}
	manager := tools.NewToolManager(nil)
	return skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
}

func toolCallPlanningResponse(toolName string, arguments string) *adapter.ChatResponse {
	return manyToolCallPlanningResponse(toolName, arguments, 1)
}

func manyToolCallPlanningResponse(toolName string, arguments string, count int) *adapter.ChatResponse {
	calls := make([]adapter.ToolCall, 0, count)
	for idx := 0; idx < count; idx++ {
		calls = append(calls, adapter.ToolCall{
			ID:   fmt.Sprintf("call_%d", idx+1),
			Type: "function",
			Function: adapter.FunctionCall{
				Name:      toolName,
				Arguments: arguments,
			},
		})
	}
	return &adapter.ChatResponse{
		Choices: []adapter.Choice{{
			Message: adapter.Message{
				Role:      "assistant",
				ToolCalls: calls,
			},
		}},
	}
}

func containsTool(tools []adapter.Tool, name string) bool {
	for _, tool := range tools {
		if tool.Function.Name == name {
			return true
		}
	}
	return false
}

func requestContainsToolResult(req *adapter.ChatRequest, callID string) bool {
	if req == nil {
		return false
	}
	for _, msg := range req.Messages {
		if msg.Role == "tool" && msg.ToolCallID == callID {
			return true
		}
	}
	return false
}

func requestMessagesContain(req *adapter.ChatRequest, text string) bool {
	if req == nil {
		return false
	}
	for _, msg := range req.Messages {
		if strings.Contains(fmt.Sprint(msg.Content), text) {
			return true
		}
	}
	return false
}

func firstEventPayload(events []aichatservice.StreamEvent, eventType string) map[string]interface{} {
	for _, event := range events {
		if event.EventType == eventType {
			return event.Payload
		}
	}
	return nil
}

func eventCount(events []aichatservice.StreamEvent, eventType string) int {
	count := 0
	for _, event := range events {
		if event.EventType == eventType {
			count++
		}
	}
	return count
}

func generatedFilesCount(value interface{}) int {
	switch typed := value.(type) {
	case []map[string]interface{}:
		return len(typed)
	case []interface{}:
		return len(typed)
	default:
		return 0
	}
}

func firstGeneratedFile(value interface{}) map[string]interface{} {
	files := generatedFilesAsMaps(value)
	if len(files) == 0 {
		return map[string]interface{}{}
	}
	return files[0]
}

func generatedFilesAsMaps(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		return typed
	case []interface{}:
		files := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if file, ok := item.(map[string]interface{}); ok {
				files = append(files, file)
			}
		}
		return files
	default:
		return nil
	}
}

func findMessageMetadata(messages []*aichatmodel.Message, id uuid.UUID) map[string]interface{} {
	for _, message := range messages {
		if message != nil && message.ID == id {
			return message.Metadata
		}
	}
	return map[string]interface{}{}
}

func firstInvocationArguments(value interface{}, skillID string, toolName string) map[string]interface{} {
	invocations, ok := value.([]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	for _, item := range invocations {
		invocation, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if metadataString(invocation["skill_id"]) != skillID || metadataString(invocation["tool_name"]) != toolName {
			continue
		}
		args, _ := invocation["arguments"].(map[string]interface{})
		return args
	}
	return map[string]interface{}{}
}

func lastInvocation(value interface{}, skillID string, toolName string) map[string]interface{} {
	invocations, ok := value.([]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	result := map[string]interface{}{}
	for _, item := range invocations {
		invocation, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if metadataString(invocation["skill_id"]) == skillID && metadataString(invocation["tool_name"]) == toolName {
			result = invocation
		}
	}
	return result
}

type aichatMemoryStorage struct {
	mu    sync.RWMutex
	files map[string][]byte
}

func newAIChatMemoryStorage() *aichatMemoryStorage {
	return &aichatMemoryStorage{files: make(map[string][]byte)}
}

func (m *aichatMemoryStorage) Save(filename string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[filename] = append([]byte(nil), data...)
	return nil
}

func (m *aichatMemoryStorage) Load(filename string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]byte(nil), m.files[filename]...), nil
}

func (m *aichatMemoryStorage) LoadStream(filename string) (<-chan []byte, error) {
	ch := make(chan []byte, 1)
	data, _ := m.Load(filename)
	ch <- data
	close(ch)
	return ch, nil
}

func (m *aichatMemoryStorage) Download(filename string, targetPath string) error {
	data, _ := m.Load(filename)
	return os.WriteFile(targetPath, data, 0o600)
}

func (m *aichatMemoryStorage) Exists(filename string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.files[filename]
	return ok, nil
}

func (m *aichatMemoryStorage) Delete(filename string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, filename)
	return nil
}

func (m *aichatMemoryStorage) List(prefix string) ([]storage.FileInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]storage.FileInfo, 0)
	for key, value := range m.files {
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			continue
		}
		result = append(result, storage.FileInfo{Key: key, Size: int64(len(value))})
	}
	return result, nil
}

func setupAIChatToolFileGlobals(t *testing.T) {
	t.Helper()

	previousManager := tool_file.GlobalToolFileManager
	previousSignature := tool_file.GlobalFileSignature
	t.Cleanup(func() {
		tool_file.GlobalToolFileManager = previousManager
		tool_file.GlobalFileSignature = previousSignature
	})

	dbPath := filepath.Join(t.TempDir(), "aichat_tool_file_test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	if err := db.AutoMigrate(&tool_file.ToolFile{}); err != nil {
		t.Fatalf("auto migrate tool_files: %v", err)
	}

	tool_file.GlobalToolFileManager = tool_file.NewToolFileManager(db, newAIChatMemoryStorage())
	tool_file.GlobalFileSignature = tool_file.NewFileSignature(&appconfig.Config{
		App: appconfig.AppConfig{
			SecretKey:          "test-secret",
			FilesURL:           "https://api.zgi.im",
			FilesAccessTimeout: 3600,
		},
	})
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
