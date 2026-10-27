package relationshipmemory_test

import (
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/agentmemoryruntime"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
)

// The chat runtime receives the adapter as an untyped dependency and finds
// these interfaces by type assertion, so a missing method would turn Agent
// memory off without a compile error.
var (
	_ runtimeservice.AgentMemoryContextService       = (*relationshipmemory.AgentAdapter)(nil)
	_ agentmemoryruntime.MemoryService               = (*relationshipmemory.AgentAdapter)(nil)
	_ agentmemoryruntime.RelationshipSettingsService = (*relationshipmemory.AgentAdapter)(nil)
)
