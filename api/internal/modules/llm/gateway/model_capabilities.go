package gateway

import (
	"context"
	"errors"
	"os"
	"strings"

	"gorm.io/gorm"

	"github.com/zgiai/zgi/api/internal/modules/llm/gateway/types"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
)

// CapabilityGateEnv disables the chat capability gate when set to 0/false/off.
// The gate is on by default: a request that asks for a capability the model
// never declared must be refused before it reaches an upstream, because the
// upstream's refusal arrives as a retryable 5xx after a channel-wide retry and
// loses the reason.
const CapabilityGateEnv = "LLM_CAPABILITY_GATE"

// CapabilityGateEnabled reports whether the request-time capability gate is
// active. Deployments whose model metadata is not maintained yet can turn it off
// without a code change; an unset value keeps the gate on.
func CapabilityGateEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(CapabilityGateEnv))) {
	case "0", "false", "off", "disabled", "no":
		return false
	default:
		return true
	}
}

// ModelCapabilities returns the capability declaration of a model for the
// request-time gate.
//
// Multiple live rows can exist for the same logical model (one per provider), so
// capabilities are unioned: a route that can serve the capability keeps the
// request routable. A missing row, an unreadable row or an unconfigured database
// yields Known=false, and callers must treat that as unknown and fail open
// rather than reject a request the metadata cannot describe.
func (s *llmGatewayServiceImpl) ModelCapabilities(
	ctx context.Context,
	model string,
) (types.ModelCapabilities, error) {
	name := normalizeRequestedModelName(model)
	if name == "" || s == nil || s.db == nil {
		return types.ModelCapabilities{}, nil
	}

	var models []llmmodel.LLMModel
	err := s.db.WithContext(ctx).
		Where("name = ? AND is_active = ?", name, true).
		Find(&models).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return types.ModelCapabilities{}, nil
	}
	if err != nil {
		return types.ModelCapabilities{}, err
	}
	if len(models) == 0 {
		return types.ModelCapabilities{}, nil
	}

	result := types.ModelCapabilities{Known: true}
	for _, m := range models {
		result.StructuredOutput = result.StructuredOutput || m.SupportsStructuredOutput
		result.JSONMode = result.JSONMode || m.SupportsJsonMode
		result.ToolCall = result.ToolCall || m.SupportsToolCall
		result.Vision = result.Vision || m.SupportsVision
		result.Rerank = result.Rerank || m.IsRerank()
		result.UseCasesDeclared = result.UseCasesDeclared || len(m.UseCases) > 0
	}
	return result, nil
}
