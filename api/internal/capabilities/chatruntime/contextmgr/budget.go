package contextmgr

import (
	"fmt"
	"math"
)

const (
	defaultAgentWindowK      = 256
	defaultSummaryOutput     = 20_000
	defaultEmergencyBuffer   = 13_000
	defaultHysteresis        = 8_000
	defaultMaxToolResult     = 32_000
	defaultToolPreviewRunes  = 12_000
	defaultTailMinTextRounds = 3
)

func normalizeConfig(config Config) (Config, error) {
	if config.ConfiguredAgentWindowK == 0 {
		config.ConfiguredAgentWindowK = defaultAgentWindowK
	}
	if config.ConfiguredAgentWindowK < 0 || config.ConfiguredAgentWindowK > int(^uint(0)>>1)/1000 {
		return Config{}, fmt.Errorf("agent context window k must be a positive non-overflowing integer")
	}
	if config.ModelContextWindow <= 0 {
		return Config{}, fmt.Errorf("model physical context window must be positive")
	}
	if config.SummaryOutputTokens <= 0 {
		config.SummaryOutputTokens = defaultSummaryOutput
	}
	if config.MaxOutputTokens > 0 && config.SummaryOutputTokens > config.MaxOutputTokens {
		config.SummaryOutputTokens = config.MaxOutputTokens
	}
	if config.EmergencyBufferTokens <= 0 {
		config.EmergencyBufferTokens = defaultEmergencyBuffer
	}
	if config.HysteresisTokens <= 0 {
		config.HysteresisTokens = defaultHysteresis
	}
	if config.TargetRatio <= 0 || config.TargetRatio >= 1 {
		config.TargetRatio = 0.60
	}
	if config.MaxToolResultTokens <= 0 {
		config.MaxToolResultTokens = defaultMaxToolResult
	}
	if config.ToolResultPreviewRunes <= 0 {
		config.ToolResultPreviewRunes = defaultToolPreviewRunes
	}
	if config.TailMinTextRounds <= 0 {
		config.TailMinTextRounds = defaultTailMinTextRounds
	}
	return config, nil
}

func budgetForRequest(config Config, requestedOutput int) (Budget, error) {
	configuredTokens := config.ConfiguredAgentWindowK * 1000
	agentWindow := min(configuredTokens, config.ModelContextWindow)
	clamped := agentWindow < configuredTokens
	mainOutput := requestedOutput
	if mainOutput <= 0 {
		mainOutput = config.DefaultMainOutputTokens
	}
	if mainOutput <= 0 {
		mainOutput = config.MaxOutputTokens
	}
	if config.MaxOutputTokens > 0 && mainOutput > config.MaxOutputTokens {
		mainOutput = config.MaxOutputTokens
	}
	if mainOutput <= 0 {
		mainOutput = min(16_000, agentWindow/8)
	}
	promptBudget := agentWindow - mainOutput
	if config.MaxInputTokens > 0 {
		promptBudget = min(promptBudget, config.MaxInputTokens)
	}
	compactInputLimit := agentWindow - config.SummaryOutputTokens
	softLimit := min(promptBudget-config.EmergencyBufferTokens, compactInputLimit-config.EmergencyBufferTokens)
	hardLimit := promptBudget
	targetTokens := min(softLimit-config.HysteresisTokens, int(math.Floor(float64(promptBudget)*config.TargetRatio)))
	if promptBudget <= 0 || compactInputLimit <= config.EmergencyBufferTokens || softLimit <= 0 || targetTokens <= 0 {
		return Budget{}, fmt.Errorf("agent context working window is too small for prompt and compaction reserves")
	}
	tailMin := clamp(int(float64(promptBudget)*0.05), 4_000, 10_000)
	tailMax := clamp(int(float64(promptBudget)*0.20), 16_000, 40_000)
	if tailMin > targetTokens/2 {
		tailMin = max(1_000, targetTokens/2)
	}
	if tailMax < tailMin {
		tailMax = tailMin
	}
	return Budget{
		ModelContextWindow:        config.ModelContextWindow,
		ConfiguredAgentWindowK:    config.ConfiguredAgentWindowK,
		AgentContextWindow:        agentWindow,
		AgentContextWindowClamped: clamped,
		MainOutputReserve:         mainOutput,
		SummaryOutputReserve:      config.SummaryOutputTokens,
		PromptBudget:              promptBudget,
		CompactInputLimit:         compactInputLimit,
		SoftLimit:                 softLimit,
		HardLimit:                 hardLimit,
		TargetTokens:              targetTokens,
		TailMinTokens:             tailMin,
		TailMaxTokens:             tailMax,
	}, nil
}

func clamp(value, lower, upper int) int {
	if value < lower {
		return lower
	}
	if value > upper {
		return upper
	}
	return value
}
