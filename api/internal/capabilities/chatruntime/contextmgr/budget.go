package contextmgr

import (
	"fmt"
)

const (
	DefaultModelContextWindowTokens   = 512_000
	defaultSummaryOutput              = 20_000
	defaultMaxToolResult              = 32_000
	DefaultCompressedToolResultTokens = 10_000
	defaultToolPreviewRunes           = 12_000
	defaultProjectToolResult          = 16_000
	defaultPreservedRounds            = 4
	contextTriggerNumerator           = 80
	contextTriggerDenominator         = 100
)

func normalizeConfig(config Config) (Config, error) {
	if config.ModelContextWindow <= 0 {
		config.ModelContextWindow = DefaultModelContextWindowTokens
	}
	if config.MaxToolResultTokens <= 0 {
		config.MaxToolResultTokens = defaultMaxToolResult
	}
	if config.ToolResultPreviewRunes <= 0 {
		config.ToolResultPreviewRunes = defaultToolPreviewRunes
	}
	if config.ProjectToolResultTokens <= 0 {
		config.ProjectToolResultTokens = defaultProjectToolResult
	}
	return config, nil
}

func budgetForRequest(config Config, requestedOutput int) (Budget, error) {
	modelWindow := config.ModelContextWindow
	summaryOutput := defaultSummaryOutput
	if config.MaxOutputTokens > 0 && summaryOutput > config.MaxOutputTokens {
		summaryOutput = config.MaxOutputTokens
	}
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
		mainOutput = min(16_000, modelWindow/8)
	}
	promptBudget := modelWindow * contextTriggerNumerator / contextTriggerDenominator
	if config.MaxInputTokens > 0 {
		promptBudget = min(promptBudget, config.MaxInputTokens)
	}
	compactInputLimit := modelWindow - summaryOutput
	if promptBudget <= 0 || compactInputLimit <= 0 {
		return Budget{}, fmt.Errorf("model context window is too small for prompt and compaction output")
	}
	return Budget{
		ModelContextWindow:   config.ModelContextWindow,
		AgentContextWindow:   modelWindow,
		MainOutputReserve:    mainOutput,
		SummaryOutputReserve: summaryOutput,
		PromptBudget:         promptBudget,
		CompactInputLimit:    compactInputLimit,
		SoftLimit:            promptBudget,
		HardLimit:            promptBudget,
		TargetTokens:         promptBudget,
	}, nil
}
