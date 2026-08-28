package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/contextmgr"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/tokenestimate"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const contextPromptDumpDirectory = "storage/debug/llm-context"

var contextPromptDumpMu sync.Mutex

func (s *service) writeAgentContextPromptDumpBestEffort(ctx context.Context, prepared *PreparedChat, requestType string, round int, request *adapter.ChatRequest, decision *contextmgr.Decision) {
	if runningUnderGoTest() || !contextPromptDumpEnabled() || prepared == nil || request == nil {
		return
	}
	path, err := writeAgentContextPromptDump(contextPromptDumpPath(), time.Now(), requestType, round, decision, request)
	if err != nil {
		logger.WarnContext(ctx, "failed to write llm context prompt dump", err)
		return
	}
	logger.InfoContext(ctx, "wrote llm context prompt dump", "path", path)
}

func contextPromptDumpEnabled() bool {
	return config.GlobalConfig != nil && config.GlobalConfig.ChatRuntime.ContextPromptDumpEnabled
}

func contextPromptDumpPath() string {
	directory, err := os.Getwd()
	if err != nil {
		return contextPromptDumpDirectory
	}
	for {
		if _, statErr := os.Stat(filepath.Join(directory, "go.mod")); statErr == nil {
			return filepath.Join(directory, contextPromptDumpDirectory)
		}
		if _, statErr := os.Stat(filepath.Join(directory, "api", "go.mod")); statErr == nil {
			return filepath.Join(directory, "api", contextPromptDumpDirectory)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return contextPromptDumpDirectory
		}
		directory = parent
	}
}

func runningUnderGoTest() bool {
	return strings.HasSuffix(filepath.Base(os.Args[0]), ".test")
}

func writeAgentContextPromptDump(directory string, now time.Time, requestType string, round int, decision *contextmgr.Decision, request *adapter.ChatRequest) (string, error) {
	if request == nil {
		return "", fmt.Errorf("context prompt dump request is required")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", fmt.Errorf("create context prompt dump directory: %w", err)
	}
	content, err := renderAgentContextPromptDump(now, requestType, round, decision, request)
	if err != nil {
		return "", err
	}

	contextPromptDumpMu.Lock()
	defer contextPromptDumpMu.Unlock()
	sequence, err := nextContextPromptDumpSequence(directory)
	if err != nil {
		return "", err
	}
	for {
		path := filepath.Join(directory, fmt.Sprintf("%06d.txt", sequence))
		file, openErr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(openErr) {
			sequence++
			continue
		}
		if openErr != nil {
			return "", fmt.Errorf("create context prompt dump: %w", openErr)
		}
		if _, writeErr := file.WriteString(content); writeErr != nil {
			_ = file.Close()
			return "", fmt.Errorf("write context prompt dump: %w", writeErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			return "", fmt.Errorf("close context prompt dump: %w", closeErr)
		}
		return path, nil
	}
}

func nextContextPromptDumpSequence(directory string) (int, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, fmt.Errorf("read context prompt dump directory: %w", err)
	}
	maxSequence := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".txt" {
			continue
		}
		sequence, parseErr := strconv.Atoi(strings.TrimSuffix(entry.Name(), ".txt"))
		if parseErr == nil && sequence > maxSequence {
			maxSequence = sequence
		}
	}
	return maxSequence + 1, nil
}

func renderAgentContextPromptDump(now time.Time, requestType string, round int, decision *contextmgr.Decision, request *adapter.ChatRequest) (string, error) {
	completeRequest, err := completeChatRequestJSON(request)
	if err != nil {
		return "", fmt.Errorf("encode complete chat request: %w", err)
	}
	metrics := effectiveDumpDecision(decision, request)

	var builder strings.Builder
	separator := strings.Repeat("=", 80)
	builder.WriteString(separator + "\n")
	fmt.Fprintf(&builder, "AgentRunID: %s\n", metrics.AgentRunID)
	fmt.Fprintf(&builder, "API Round: %d\n", firstPositive(metrics.APIRound, round))
	fmt.Fprintf(&builder, "请求类型: %s\n", firstNonEmptyString(metrics.RequestType, requestType, contextmgr.RequestTypeMain))
	fmt.Fprintf(&builder, "模型: %s\n", request.Model)
	fmt.Fprintf(&builder, "时间: %s\n\n", now.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&builder, "模型物理上下文窗口: %d\n", metrics.Budget.ModelContextWindow)
	fmt.Fprintf(&builder, "配置 Agent 工作窗口(k): %d\n", metrics.Budget.ConfiguredAgentWindowK)
	fmt.Fprintf(&builder, "有效 Agent 工作窗口 Token: %d\n", metrics.Budget.AgentContextWindow)
	fmt.Fprintf(&builder, "工作窗口是否按模型物理窗口裁剪: %s\n", chineseYesNo(metrics.Budget.AgentContextWindowClamped))
	fmt.Fprintf(&builder, "PromptBudget: %d\n", metrics.Budget.PromptBudget)
	fmt.Fprintf(&builder, "固定请求 Token: %d\n", metrics.FixedRequestTokens)
	fmt.Fprintf(&builder, "可压缩区 Token: %d\n", metrics.CompressibleTokens)
	fmt.Fprintf(&builder, "最终 Prompt Token: %d\n", metrics.FinalPromptTokens)
	fmt.Fprintf(&builder, "总上下文压力: %.2f%%\n", metrics.Budget.ContextPressure*100)
	fmt.Fprintf(&builder, "SoftLimit: %d\n", metrics.Budget.SoftLimit)
	fmt.Fprintf(&builder, "HardLimit: %d\n", metrics.Budget.HardLimit)
	fmt.Fprintf(&builder, "TargetTokens: %d\n\n", metrics.Budget.TargetTokens)
	fmt.Fprintf(&builder, "本次决策: %s\n", metrics.Action)
	fmt.Fprintf(&builder, "压缩前 Token: %d\n", metrics.BeforeTokens)
	fmt.Fprintf(&builder, "压缩后 Token: %d\n", metrics.AfterTokens)
	fmt.Fprintf(&builder, "压缩覆盖至 API Round: %d\n", metrics.CompactedThroughRound)
	fmt.Fprintf(&builder, "保留原文 API Rounds: %v\n", metrics.PreservedRounds)
	fmt.Fprintf(&builder, "工具结果原始 Token: %d\n", metrics.ToolResultOriginalTokens)
	fmt.Fprintf(&builder, "工具结果投影后 Token: %d\n", metrics.ToolResultProjectedTokens)
	fmt.Fprintf(&builder, "摘要输入裁剪 API Rounds: %d\n", metrics.LossyRecoveryDroppedRounds)
	fmt.Fprintf(&builder, "估算器/校准系数: %s / %.4f\n", metrics.Estimator, metrics.EstimateScale)
	if metrics.CompactionFailure != "" {
		fmt.Fprintf(&builder, "压缩失败: %s\n", metrics.CompactionFailure)
	}
	builder.WriteString(separator + "\n\n")
	builder.WriteString("【最终完整 ChatRequest】\n\n")
	builder.WriteString(completeRequest)
	builder.WriteString("\n\n" + separator + "\n")
	return builder.String(), nil
}

func effectiveDumpDecision(decision *contextmgr.Decision, request *adapter.ChatRequest) contextmgr.Decision {
	if decision != nil {
		cloned := *decision
		return cloned
	}
	estimate := tokenestimate.NewEstimator().EstimateChatRequest(request)
	return contextmgr.Decision{Action: contextmgr.DecisionNone, FinalPromptTokens: estimate.Tokens, BeforeTokens: estimate.Tokens, AfterTokens: estimate.Tokens, Estimator: estimate.Tokenizer, EstimateScale: 1}
}

func completeChatRequestJSON(request *adapter.ChatRequest) (string, error) {
	encoded, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	view := map[string]interface{}{}
	if err := json.Unmarshal(encoded, &view); err != nil {
		return "", err
	}
	view["provider"] = request.Provider
	if len(request.AdditionalParameters) > 0 {
		view["additional_parameters"] = redactPromptValue(request.AdditionalParameters)
	}
	encoded, err = json.MarshalIndent(view, "", "  ")
	return string(encoded), err
}

func redactPromptValue(value interface{}) interface{} {
	sensitive := func(key string) bool {
		key = strings.ToLower(strings.TrimSpace(key))
		return strings.Contains(key, "authorization") ||
			strings.Contains(key, "api_key") ||
			strings.Contains(key, "apikey") ||
			strings.Contains(key, "password") ||
			strings.Contains(key, "cookie") ||
			strings.Contains(key, "secret") ||
			strings.Contains(key, "access_token") ||
			strings.Contains(key, "refresh_token") ||
			strings.Contains(key, "bearer_token") ||
			strings.Contains(key, "credential")
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			if sensitive(key) {
				out[key] = "[REDACTED]"
			} else {
				out[key] = redactPromptValue(item)
			}
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(typed))
		for index, item := range typed {
			out[index] = redactPromptValue(item)
		}
		return out
	default:
		return value
	}
}

func chineseYesNo(value bool) string {
	if value {
		return "是"
	}
	return "否"
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
