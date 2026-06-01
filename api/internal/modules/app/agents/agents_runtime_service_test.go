package agents

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestNormalizeAgentEnabledSkillIDsRemovesRuntimeManagedSkills(t *testing.T) {
	got := normalizeAgentEnabledSkillIDs([]string{
		skills.SkillAgentKnowledge,
		skills.SkillUserMemory,
		skills.SkillCalculator,
		skills.SkillCalculator,
		"  time  ",
		"",
	})
	want := []string{skills.SkillCalculator, skills.SkillTime}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeAgentEnabledSkillIDs() = %#v, want %#v", got, want)
	}
}

func TestAgentMemoryReplaceRequestPreservesInvalidRowsForValidation(t *testing.T) {
	req := agentMemoryReplaceRequestFromConfig([]dto.AgentMemorySlotConfig{
		{Key: "profile", Enabled: true},
		{Key: "", Enabled: true},
		{Key: "profile", Enabled: true},
	}, true)
	if len(req.Slots) != 3 {
		t.Fatalf("len(req.Slots) = %d, want 3", len(req.Slots))
	}
	if req.Slots[1].Key != "" {
		t.Fatalf("empty key row was changed to %q", req.Slots[1].Key)
	}
	if req.Slots[2].Key != "profile" {
		t.Fatalf("duplicate key row was changed to %q", req.Slots[2].Key)
	}
}

func TestAgentMemoryReplaceRequestCanDropHistoricalIDsForRollback(t *testing.T) {
	req := agentMemoryReplaceRequestFromConfig([]dto.AgentMemorySlotConfig{
		{ID: "stale-slot-id", Key: "profile", Enabled: true},
	}, false)
	if len(req.Slots) != 1 {
		t.Fatalf("len(req.Slots) = %d, want 1", len(req.Slots))
	}
	if req.Slots[0].ID != "" {
		t.Fatalf("rollback request preserved historical id %q", req.Slots[0].ID)
	}
	if req.Slots[0].Key != "profile" {
		t.Fatalf("rollback request key = %q, want profile", req.Slots[0].Key)
	}
}

func TestAgentMemorySnapshotSlotsDoNotPersistVolatileIDs(t *testing.T) {
	slots := agentMemorySnapshotSlots([]dto.AgentMemorySlotConfig{
		{
			ID:          "draft-slot-id",
			Key:         "profile",
			Description: "User profile",
			MaxChars:    4000,
			Enabled:     true,
			SortOrder:   3,
			CreatedAt:   123,
			UpdatedAt:   456,
		},
	})
	if len(slots) != 1 {
		t.Fatalf("len(slots) = %d, want 1", len(slots))
	}
	if slots[0].ID != "" {
		t.Fatalf("snapshot preserved slot id %q", slots[0].ID)
	}
	if slots[0].CreatedAt != 0 || slots[0].UpdatedAt != 0 {
		t.Fatalf("snapshot preserved timestamps created=%d updated=%d", slots[0].CreatedAt, slots[0].UpdatedAt)
	}
	if slots[0].MaxChars != 2000 {
		t.Fatalf("snapshot max chars = %d, want 2000", slots[0].MaxChars)
	}
}

func TestApplyAgentConfigRequestToDraftDoesNotPersistDraftMemorySlots(t *testing.T) {
	cfg := &AgentsConfig{}
	applied, err := applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{
		AgentMemoryEnabled: true,
	})
	if err != nil {
		t.Fatalf("applyAgentConfigRequestToDraft() error = %v", err)
	}
	if !applied.AgentMemoryEnabled {
		t.Fatal("applied AgentMemoryEnabled = false, want true")
	}
	if cfg.AgentMode == nil {
		t.Fatal("AgentMode = nil, want serialized runtime mode")
	}
	if strings.Contains(*cfg.AgentMode, "agent_memory_slots") {
		t.Fatalf("AgentMode = %s, should not persist draft memory slots", *cfg.AgentMode)
	}
	var mode dto.AgentRuntimeModeConfig
	if err := json.Unmarshal([]byte(*cfg.AgentMode), &mode); err != nil {
		t.Fatalf("unmarshal AgentMode error = %v", err)
	}
	if !mode.AgentMemoryEnabled {
		t.Fatal("mode AgentMemoryEnabled = false, want true")
	}
}

func TestApplyAgentConfigRequestPersistsDatabaseBindings(t *testing.T) {
	cfg := &AgentsConfig{}
	applied, err := applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{
		DatabaseBindings: []dto.AgentDatabaseBinding{
			{DataSourceID: " db-1 ", TableIDs: []string{"table-2", "table-1", "table-1"}, WritableTableIDs: []string{"table-2", "table-missing", "table-2"}},
			{DataSourceID: "db-empty", TableIDs: []string{}},
		},
	})
	if err != nil {
		t.Fatalf("applyAgentConfigRequestToDraft() error = %v", err)
	}
	want := []dto.AgentDatabaseBinding{{DataSourceID: "db-1", TableIDs: []string{"table-1", "table-2"}, WritableTableIDs: []string{"table-2"}}}
	if !reflect.DeepEqual(applied.DatabaseBindings, want) {
		t.Fatalf("applied DatabaseBindings = %#v, want %#v", applied.DatabaseBindings, want)
	}
	if cfg.AgentMode == nil {
		t.Fatal("AgentMode = nil, want serialized runtime mode")
	}
	var mode dto.AgentRuntimeModeConfig
	if err := json.Unmarshal([]byte(*cfg.AgentMode), &mode); err != nil {
		t.Fatalf("unmarshal AgentMode error = %v", err)
	}
	if !reflect.DeepEqual(mode.DatabaseBindings, want) {
		t.Fatalf("mode DatabaseBindings = %#v, want %#v", mode.DatabaseBindings, want)
	}
	resp := agentConfigResponse("agent-1", cfg)
	if !reflect.DeepEqual(resp.DatabaseBindings, want) {
		t.Fatalf("response DatabaseBindings = %#v, want %#v", resp.DatabaseBindings, want)
	}
	snapshot := agentConfigSnapshot("agent-1", cfg)
	fromSnapshot := agentConfigResponseFromSnapshot("agent-1", snapshot)
	if !reflect.DeepEqual(fromSnapshot.DatabaseBindings, want) {
		t.Fatalf("snapshot DatabaseBindings = %#v, want %#v", fromSnapshot.DatabaseBindings, want)
	}
}

func TestApplyAgentConfigRequestRecordsBindingActor(t *testing.T) {
	cfg := &AgentsConfig{}
	_, err := applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{
		KnowledgeDatasetIDs: []string{"dataset-1"},
		DatabaseBindings: []dto.AgentDatabaseBinding{
			{DataSourceID: "db-1", TableIDs: []string{"table-1"}},
		},
	}, "binder-1")
	if err != nil {
		t.Fatalf("applyAgentConfigRequestToDraft() error = %v", err)
	}
	var mode dto.AgentRuntimeModeConfig
	if err := json.Unmarshal([]byte(*cfg.AgentMode), &mode); err != nil {
		t.Fatalf("unmarshal AgentMode error = %v", err)
	}
	if mode.KnowledgeBoundByAccountID != "binder-1" {
		t.Fatalf("KnowledgeBoundByAccountID = %q, want binder-1", mode.KnowledgeBoundByAccountID)
	}
	if mode.KnowledgeBoundAtUnix <= 0 {
		t.Fatalf("KnowledgeBoundAtUnix = %d, want positive", mode.KnowledgeBoundAtUnix)
	}
	if mode.DatabaseBoundByAccountID != "binder-1" {
		t.Fatalf("DatabaseBoundByAccountID = %q, want binder-1", mode.DatabaseBoundByAccountID)
	}
	if mode.DatabaseBoundAtUnix <= 0 {
		t.Fatalf("DatabaseBoundAtUnix = %d, want positive", mode.DatabaseBoundAtUnix)
	}
	snapshot := agentConfigSnapshot("agent-1", cfg)
	fromSnapshot := agentConfigResponseFromSnapshot("agent-1", snapshot)
	if fromSnapshot.KnowledgeBoundByAccountID != "binder-1" || fromSnapshot.DatabaseBoundByAccountID != "binder-1" || fromSnapshot.KnowledgeBoundAtUnix <= 0 || fromSnapshot.DatabaseBoundAtUnix <= 0 {
		t.Fatalf("snapshot binding actors: knowledge=%q/%d database=%q/%d, want binder-1 with timestamps", fromSnapshot.KnowledgeBoundByAccountID, fromSnapshot.KnowledgeBoundAtUnix, fromSnapshot.DatabaseBoundByAccountID, fromSnapshot.DatabaseBoundAtUnix)
	}
	firstKnowledgeBoundAt := mode.KnowledgeBoundAtUnix
	firstDatabaseBoundAt := mode.DatabaseBoundAtUnix

	_, err = applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{
		KnowledgeDatasetIDs: []string{"dataset-1"},
		DatabaseBindings: []dto.AgentDatabaseBinding{
			{DataSourceID: "db-1", TableIDs: []string{"table-1"}},
		},
		SystemPrompt: "updated prompt only",
	}, "editor-2")
	if err != nil {
		t.Fatalf("second applyAgentConfigRequestToDraft() error = %v", err)
	}
	if err := json.Unmarshal([]byte(*cfg.AgentMode), &mode); err != nil {
		t.Fatalf("unmarshal updated AgentMode error = %v", err)
	}
	if mode.KnowledgeBoundByAccountID != "binder-1" || mode.DatabaseBoundByAccountID != "binder-1" || mode.KnowledgeBoundAtUnix != firstKnowledgeBoundAt || mode.DatabaseBoundAtUnix != firstDatabaseBoundAt {
		t.Fatalf("binding grants changed on unrelated save: knowledge=%q/%d database=%q/%d", mode.KnowledgeBoundByAccountID, mode.KnowledgeBoundAtUnix, mode.DatabaseBoundByAccountID, mode.DatabaseBoundAtUnix)
	}

	_, err = applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{
		KnowledgeDatasetIDs: []string{"dataset-1", "dataset-2"},
		DatabaseBindings: []dto.AgentDatabaseBinding{
			{DataSourceID: "db-1", TableIDs: []string{"table-1", "table-2"}},
		},
	}, "editor-2")
	if err != nil {
		t.Fatalf("third applyAgentConfigRequestToDraft() error = %v", err)
	}
	if err := json.Unmarshal([]byte(*cfg.AgentMode), &mode); err != nil {
		t.Fatalf("unmarshal changed AgentMode error = %v", err)
	}
	if mode.KnowledgeBoundByAccountID != "editor-2" || mode.DatabaseBoundByAccountID != "editor-2" {
		t.Fatalf("binding actors after binding change: knowledge=%q database=%q, want editor-2", mode.KnowledgeBoundByAccountID, mode.DatabaseBoundByAccountID)
	}
}

func TestApplyAgentConfigRequestCanRestoreSnapshotBindingGrant(t *testing.T) {
	cfg := &AgentsConfig{}
	_, err := applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{
		KnowledgeDatasetIDs:       []string{"dataset-1"},
		KnowledgeBoundByAccountID: "snapshot-binder",
		KnowledgeBoundAtUnix:      123,
		DatabaseBindings: []dto.AgentDatabaseBinding{
			{DataSourceID: "db-1", TableIDs: []string{"table-1"}, WritableTableIDs: []string{"table-1"}},
		},
		DatabaseBoundByAccountID: "snapshot-binder",
		DatabaseBoundAtUnix:      456,
	}, "rollback-user")
	if err != nil {
		t.Fatalf("applyAgentConfigRequestToDraft() error = %v", err)
	}
	var mode dto.AgentRuntimeModeConfig
	if err := json.Unmarshal([]byte(*cfg.AgentMode), &mode); err != nil {
		t.Fatalf("unmarshal AgentMode error = %v", err)
	}
	if mode.KnowledgeBoundByAccountID != "snapshot-binder" || mode.KnowledgeBoundAtUnix != 123 {
		t.Fatalf("knowledge grant = %q/%d, want snapshot-binder/123", mode.KnowledgeBoundByAccountID, mode.KnowledgeBoundAtUnix)
	}
	if mode.DatabaseBoundByAccountID != "snapshot-binder" || mode.DatabaseBoundAtUnix != 456 {
		t.Fatalf("database grant = %q/%d, want snapshot-binder/456", mode.DatabaseBoundByAccountID, mode.DatabaseBoundAtUnix)
	}
	if len(mode.DatabaseBindings) != 1 || !reflect.DeepEqual(mode.DatabaseBindings[0].WritableTableIDs, []string{"table-1"}) {
		t.Fatalf("database bindings = %#v, want writable table-1", mode.DatabaseBindings)
	}
}
