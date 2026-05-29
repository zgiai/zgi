package service

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestEffectiveAgentSkillIDsAutoAddsHiddenKnowledge(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}},
		{ID: skills.SkillInternalKnowledge, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillAgentKnowledge, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}, RequiredConfig: []string{skills.SkillRequiredConfigAgentKnowledge}},
		{ID: skills.SkillUserMemory, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}},
	}

	got := effectiveAgentSkillIDs(
		[]string{skills.SkillCalculator, skills.SkillAgentKnowledge, skills.SkillUserMemory, skills.SkillInternalKnowledge},
		catalog,
		&RunConfig{KnowledgeDatasetIDs: []string{"dataset-1"}},
	)
	want := []string{skills.SkillAgentKnowledge, skills.SkillCalculator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("effectiveAgentSkillIDs() = %#v, want %#v", got, want)
	}
}

func TestEffectiveAgentSkillIDsSkipsKnowledgeWithoutDatasets(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}},
		{ID: skills.SkillAgentKnowledge, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}, RequiredConfig: []string{skills.SkillRequiredConfigAgentKnowledge}},
	}

	got := effectiveAgentSkillIDs(
		[]string{skills.SkillCalculator, skills.SkillAgentKnowledge},
		catalog,
		&RunConfig{},
	)
	want := []string{skills.SkillCalculator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("effectiveAgentSkillIDs() = %#v, want %#v", got, want)
	}
}

func TestEffectiveAgentSkillIDsDoesNotAutoAddHiddenAgentMemory(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillAgentMemory, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}},
		{ID: skills.SkillUserMemory, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
	}

	got := effectiveAgentSkillIDs(
		[]string{skills.SkillUserMemory},
		catalog,
		&RunConfig{
			AgentMemoryEnabled: true,
			AgentMemorySlots: []AgentMemorySlotConfig{{
				Key:      "profile",
				MaxChars: 1000,
				Enabled:  true,
			}},
		},
	)
	want := []string{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("effectiveAgentSkillIDs() = %#v, want %#v", got, want)
	}
}

func TestRunConfigAllowsUserMemoryRejectsAgent(t *testing.T) {
	if runConfigAllowsUserMemory(RunConfig{UseMemory: true, BillingAppType: runtimemodel.ConversationCallerAgent}) {
		t.Fatal("runConfigAllowsUserMemory() = true for agent, want false")
	}
	if !runConfigAllowsUserMemory(RunConfig{UseMemory: true, BillingAppType: runtimemodel.ConversationCallerAIChat}) {
		t.Fatal("runConfigAllowsUserMemory() = false for aichat, want true")
	}
}

func TestVisibleSkillMetadataHidesRuntimeManagedSkills(t *testing.T) {
	metadata := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillInternalKnowledge},
		{ID: skills.SkillAgentKnowledge},
		{ID: skills.SkillAgentMemory},
		{ID: skills.SkillUserMemory},
		{ID: skills.SkillCalculator},
	}

	got := visibleSkillMetadata(metadata)
	gotIDs := make([]string, 0, len(got))
	for _, item := range got {
		gotIDs = append(gotIDs, item.ID)
	}
	want := []string{skills.SkillInternalKnowledge, skills.SkillCalculator}
	if !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("visibleSkillMetadata ids = %#v, want %#v", gotIDs, want)
	}
}

func TestMergeSkillTraceMetadataAppendsExistingInvocations(t *testing.T) {
	source := map[string]interface{}{
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":       "memory_planner",
				"skill_id":   skills.SkillAgentMemory,
				"tool_name":  "plan_agent_memory",
				"status":     "success_update",
				"runtime_id": "memory-planner-1",
			},
		},
	}

	metadata := mergeSkillTraceMetadata(source, []skills.SkillTrace{{
		Kind:     "tool_call",
		SkillID:  skills.SkillCalculator,
		ToolName: "calculate",
		Status:   "success",
	}})

	invocations, ok := metadata["skill_invocations"].([]interface{})
	if !ok || len(invocations) != 2 {
		t.Fatalf("skill_invocations = %#v, want two invocations", metadata["skill_invocations"])
	}
	first, _ := invocations[0].(map[string]interface{})
	second, _ := invocations[1].(map[string]interface{})
	if first["kind"] != "memory_planner" || second["tool_name"] != "calculate" {
		t.Fatalf("skill_invocations = %#v, want preserved planner then calculator tool", invocations)
	}
	if metadata["skill_step_count"] != 1 || metadata["tool_call_count"] != 1 {
		t.Fatalf("summary = %#v, want only visible calculator tool counted", metadata)
	}
}

func TestMergeSkillInvocationMetadataMergesStartAndEndByRuntimeID(t *testing.T) {
	runtimeID := "agent-memory:update:profile"
	metadata := mergeSkillInvocationMetadata(nil, []map[string]interface{}{
		newSkillInvocation("tool_call", skills.SkillAgentMemory, "update_agent_memory", "running", map[string]interface{}{
			"runtime_id": runtimeID,
			"arguments":  map[string]interface{}{"key": "profile"},
		}),
	})
	metadata = mergeSkillInvocationMetadata(metadata, []map[string]interface{}{
		newSkillInvocation("tool_call", skills.SkillAgentMemory, "update_agent_memory", "success", map[string]interface{}{
			"runtime_id": runtimeID,
			"result":     map[string]interface{}{"key": "profile"},
		}),
	})

	invocations, ok := metadata["skill_invocations"].([]interface{})
	if !ok || len(invocations) != 1 {
		t.Fatalf("skill_invocations = %#v, want one merged invocation", metadata["skill_invocations"])
	}
	invocation, _ := invocations[0].(map[string]interface{})
	if invocation["status"] != "success" || invocation["runtime_id"] != runtimeID {
		t.Fatalf("invocation = %#v, want success with runtime id", invocation)
	}
}

func TestMessageEndPayloadPreservesTraceMetadata(t *testing.T) {
	conversationID := uuid.New()
	messageID := uuid.New()
	payload := messageEndPayload(&PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: conversationID},
		Message:      &runtimemodel.Message{ID: messageID},
	}, map[string]interface{}{
		"usage": map[string]interface{}{"total_tokens": 1},
		"context_control": map[string]interface{}{
			"agent_memory": map[string]interface{}{"planner_status": "success_update"},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{"kind": "memory_planner"},
		},
		"has_trace": true,
	})

	metadata, ok := payload["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("metadata = %#v, want map", payload["metadata"])
	}
	if metadata["has_trace"] != true || metadata["skill_invocations"] == nil || metadata["context_control"] == nil {
		t.Fatalf("metadata = %#v, want trace metadata preserved", metadata)
	}
}

func TestProcessTimelineRecorderMergesSkillCallStartAndEnd(t *testing.T) {
	prepared := preparedTimelineTestChat()
	recorder := newProcessTimelineRecorder(context.Background(), context.Background(), &service{}, prepared, nil)

	recorder.RecordEvent(streamEventSkillCallStart, map[string]interface{}{
		"conversation_id":   prepared.Conversation.ID.String(),
		"message_id":        prepared.Message.ID.String(),
		"skill_id":          skills.SkillCalculator,
		"tool_name":         "calculate",
		"arguments_summary": map[string]interface{}{"expression": "1+1"},
	})
	recorder.RecordEvent(streamEventSkillCallEnd, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"skill_id":        skills.SkillCalculator,
		"tool_name":       "calculate",
		"status":          "success",
		"result":          map[string]interface{}{"value": 2},
	})

	invocation := onlyTimelineInvocation(t, prepared)
	if invocation["status"] != "success" || invocation["runtime_id"] == "" {
		t.Fatalf("invocation = %#v, want merged success with runtime_id", invocation)
	}
	if invocation["arguments"] == nil || invocation["result"] == nil {
		t.Fatalf("invocation = %#v, want arguments and result preserved", invocation)
	}
}

func TestProcessTimelineRecorderDoesNotDuplicateStreamBackedTrace(t *testing.T) {
	prepared := preparedTimelineTestChat()
	recorder := newProcessTimelineRecorder(context.Background(), context.Background(), &service{}, prepared, nil)

	recorder.RecordEvent(streamEventSkillCallStart, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"skill_id":        skills.SkillCalculator,
		"tool_name":       "calculate",
	})
	trace := skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillCalculator,
		ToolName: "calculate",
		Status:   "success",
	}
	recorder.RecordEvent(streamEventSkillCallEnd, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"skill_id":        skills.SkillCalculator,
		"tool_name":       "calculate",
		"status":          "success",
	})
	recorder.RecordTrace([]skills.SkillTrace{trace}, trace)

	invocations, ok := prepared.Message.Metadata["skill_invocations"].([]interface{})
	if !ok || len(invocations) != 1 {
		t.Fatalf("skill_invocations = %#v, want one invocation", prepared.Message.Metadata["skill_invocations"])
	}
}

func TestProcessTimelineRecorderSkipsInternalDiagnosticTrace(t *testing.T) {
	prepared := preparedTimelineTestChat()
	recorder := newProcessTimelineRecorder(context.Background(), context.Background(), &service{}, prepared, nil)
	trace := skills.SkillTrace{
		Kind:     "memory_planner",
		SkillID:  skills.SkillAgentMemory,
		ToolName: "plan_agent_memory",
		Status:   "success_update",
	}

	recorder.RecordTrace([]skills.SkillTrace{trace}, trace)

	if invocations := prepared.Message.Metadata["skill_invocations"]; invocations != nil {
		t.Fatalf("skill_invocations = %#v, want no persisted planner invocation", invocations)
	}
}

func TestProcessTimelineRecorderAggregatesIntermediateAnswerChunks(t *testing.T) {
	prepared := preparedTimelineTestChat()
	recorder := newProcessTimelineRecorder(context.Background(), context.Background(), &service{}, prepared, nil)
	basePayload := map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"answer_id":       "answer-1",
		"title":           "Draft",
		"delta":           true,
	}

	first := copyStringAnyMap(basePayload)
	first["content"] = "hello "
	first["done"] = false
	recorder.RecordEvent(streamEventIntermediateAnswer, first)
	second := copyStringAnyMap(basePayload)
	second["content"] = "world"
	second["done"] = false
	recorder.RecordEvent(streamEventIntermediateAnswer, second)
	done := copyStringAnyMap(basePayload)
	done["content"] = ""
	done["done"] = true
	recorder.RecordEvent(streamEventIntermediateAnswer, done)

	invocation := onlyTimelineInvocation(t, prepared)
	if invocation["status"] != "success" || invocation["message"] != "hello world" {
		t.Fatalf("invocation = %#v, want aggregated successful intermediate answer", invocation)
	}
}

func preparedTimelineTestChat() *PreparedChat {
	return &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message: &runtimemodel.Message{
			ID:       uuid.New(),
			Metadata: map[string]interface{}{},
		},
	}
}

func onlyTimelineInvocation(t *testing.T, prepared *PreparedChat) map[string]interface{} {
	t.Helper()
	invocations, ok := prepared.Message.Metadata["skill_invocations"].([]interface{})
	if !ok || len(invocations) != 1 {
		t.Fatalf("skill_invocations = %#v, want one invocation", prepared.Message.Metadata["skill_invocations"])
	}
	invocation, ok := invocations[0].(map[string]interface{})
	if !ok {
		t.Fatalf("invocation type = %T, want map", invocations[0])
	}
	return invocation
}
