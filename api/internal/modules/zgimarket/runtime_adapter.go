package zgimarket

import (
	"context"
	"sync/atomic"

	"github.com/google/uuid"

	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
)

// shared is the market tools of this process. The chat runtimes of agents, digital humans, and
// the external API are built in separate route groups; they all read the tools from here.
var shared atomic.Pointer[Tools]

// Share makes the market tools available to every chat runtime of the process.
func Share(tools *Tools) { shared.Store(tools) }

// RuntimeResolver lets a chat runtime list the market tools of its agent or digital human. It
// lists none until Share was called with tools that reach a gateway.
func RuntimeResolver() runtimeservice.MarketToolResolver { return runtimeResolver{} }

type runtimeResolver struct{}

func (runtimeResolver) SubjectMarketTools(
	ctx context.Context, organizationID uuid.UUID, subjectType string, subjectID uuid.UUID,
) ([]runtimeservice.MarketTool, error) {
	tools := shared.Load()
	if tools == nil {
		return nil, nil
	}
	found, err := tools.SubjectTools(ctx, organizationID, subjectType, subjectID)
	if err != nil {
		return nil, err
	}
	result := make([]runtimeservice.MarketTool, 0, len(found))
	for _, tool := range found {
		result = append(result, runtimeservice.MarketTool{
			ProviderID: tool.ProviderID, Name: tool.Name, Description: tool.Description,
			InputSchema: tool.InputSchema, RiskLevel: tool.RiskLevel,
			MaxChargeMicros: tool.MaxChargeMicros, Costly: tool.Costly,
		})
	}
	return result, nil
}
