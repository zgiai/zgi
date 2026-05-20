package indexing

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/dto"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	llmruntime "github.com/zgiai/zgi/api/internal/modules/llm/runtime"
	shared_model "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/pkg/logger"
)

var (
	resumeKeywords  = regexp.MustCompile(`(?i)(简历|工作经历|教育背景|求职意向|resume|curriculum\s*vitae|cv)`)
	invoiceKeywords = regexp.MustCompile(`(?i)(发票|增值税|开票日期|购买方|销售方|invoice|tax\s*id)`)
)

type DocDomainAnalyzer struct {
	ctx             context.Context
	llmClient       llmclient.LLMClient
	defaultModelSvc llmdefaultservice.DefaultModelService
	tenantID        string
}

func NewDocDomainAnalyzer(ctx context.Context, llmClient llmclient.LLMClient, defaultModelSvc llmdefaultservice.DefaultModelService, tenantID string) *DocDomainAnalyzer {
	return &DocDomainAnalyzer{
		ctx:             ctx,
		llmClient:       llmClient,
		defaultModelSvc: defaultModelSvc,
		tenantID:        tenantID,
	}
}

func (a *DocDomainAnalyzer) Analyze(output *dto.ExtractOutput) string {
	text := dto.ExtractOutputText(output)
	if len(text) > 1000 {
		text = text[:1000]
	}

	if resumeKeywords.MatchString(text) {
		return "resume"
	}
	if invoiceKeywords.MatchString(text) {
		return "invoice"
	}

	// LLM Fallback
	if a.llmClient == nil || a.defaultModelSvc == nil || a.tenantID == "" {
		return "unknown"
	}

	timeoutCtx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()

	resolvedModel, err := llmruntime.NewModelResolver(a.defaultModelSvc).ResolveDefault(timeoutCtx, a.tenantID, shared_model.ModelTypeLLM)
	if err != nil {
		logger.Warn("Failed to resolve default model for domain analyzer", map[string]interface{}{"error": err.Error()})
		return "unknown"
	}

	prompt := "Please analyze the following document prefix and determine its domain. Respond ONLY with one of these words: 'resume', 'invoice', or 'unknown'.\n\nDocument prefix:\n" + text

	resp, err := a.llmClient.Chat(timeoutCtx, a.tenantID, &llmadapter.ChatRequest{
		Model: resolvedModel.Model,
		Messages: []llmadapter.Message{
			{Role: "user", Content: prompt},
		},
		Stream: false,
	})

	if err != nil || resp == nil || len(resp.Choices) == 0 {
		return "unknown"
	}

	result, ok := resp.Choices[0].Message.Content.(string)
	if !ok {
		return "unknown"
	}

	result = strings.ToLower(strings.TrimSpace(result))
	if result == "resume" || result == "invoice" {
		return result
	}

	return "unknown"
}
