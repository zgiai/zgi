package memoryplanner

import (
	"context"
	"time"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	DefaultMaxPlanningRounds  = 3
	DefaultMaxPlanningRetries = 1
	DefaultMaxTokens          = 700
	DefaultPlanningTimeout    = 20 * time.Second
)

type Domain interface {
	PlannerMessages(baseMessages []adapter.Message) []adapter.Message
	ParseDecision(raw string) (interface{}, error)
	IsNoop(decision interface{}) bool
	NoopStatus(decision interface{}) string
	PlannerSuccessStatus(decision interface{}) string
	PlannerTrace(decision interface{}, status string, err error) skills.SkillTrace
	ApplyDecision(ctx context.Context, decision interface{}) (map[string]interface{}, skills.SkillTrace, error)
	SuccessNote(decision interface{}, result map[string]interface{}) adapter.Message
	GuardNote(status string) adapter.Message
	MetadataUpdates(decision interface{}, plannerStatus string, result map[string]interface{}, mutationStatus string) map[string]interface{}
}

type Request struct {
	LatestUserMessage string
	LLMRequest        *adapter.ChatRequest
	LLMClient         llmclient.LLMClient
	AppContext        *llmclient.AppContext
	UseJSONMode       bool
	MaxPlanningRounds int
	MaxRetries        int
	MaxTokens         int
	PlanningTimeout   time.Duration
}

type Result struct {
	Usage           *adapter.Usage
	Messages        []adapter.Message
	Traces          []skills.SkillTrace
	MetadataUpdates map[string]interface{}
}
