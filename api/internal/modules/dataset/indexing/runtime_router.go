package indexing

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
)

const maxFullDocParentChunkRunes = 2000

// RouterInput is the normalized input for runtime routing.
type RouterInput struct {
	DocumentID      string
	DatasetID       string
	DataSourceType  string
	DocExt          string
	ExtractedOutput *dto.ExtractOutput
}

// RouterDecision is the normalized output for runtime routing.
type RouterDecision struct {
	Matched       bool
	RouteName     string
	TargetDocForm string
	TargetMode    string
	TargetRules   map[string]interface{}
	Reason        string
	RouteMeta     map[string]interface{}
}

// RuntimeRouter performs runtime routing decisions for indexing.
type RuntimeRouter struct {
	builder         *RuntimeRuleBuilder
	domainAnalyzer  *DocDomainAnalyzer
	profileAnalyzer *DocProfileAnalyzer
}

// NewRuntimeRouter creates a runtime router.
func NewRuntimeRouter(ctx context.Context, llmClient llmclient.LLMClient, defaultModelSvc llmdefaultservice.DefaultModelService, tenantID string) *RuntimeRouter {
	return &RuntimeRouter{
		builder:         NewRuntimeRuleBuilder(),
		domainAnalyzer:  NewDocDomainAnalyzer(ctx, llmClient, defaultModelSvc, tenantID),
		profileAnalyzer: NewDocProfileAnalyzer(),
	}
}

// Route evaluates rules using normalized file extensions, domain analysis, and profile analysis.
func (r *RuntimeRouter) Route(input RouterInput) (*RouterDecision, error) {
	docExt := normalizeDocExt(input.DocExt)
	routeMeta := map[string]interface{}{
		"version":                 "v1",
		"doc_ext":                 docExt,
		"extracted_element_count": extractedElementCount(input.ExtractedOutput),
	}

	if input.DataSourceType != "upload_file" {
		return &RouterDecision{
			Matched:   false,
			Reason:    "data source type is not upload_file",
			RouteMeta: routeMeta,
		}, nil
	}

	// 1. Static Extension Routing (Table Mode)
	switch docExt {
	case ".xlsx", ".xls", ".csv":
		mode, rules := r.builder.BuildTableRule()
		routeMeta["matched_by"] = "doc_ext"

		return &RouterDecision{
			Matched:       true,
			RouteName:     "table_model",
			TargetDocForm: string(TableIndex),
			TargetMode:    mode,
			TargetRules:   rules,
			Reason:        fmt.Sprintf("matched by file extension %s", docExt),
			RouteMeta:     routeMeta,
		}, nil
	}

	// 2. Domain Analysis Routing
	extractedRunes, fullDocSizeAllowed := extractedRuneCountUpTo(input.ExtractedOutput, maxFullDocParentChunkRunes)
	routeMeta["extracted_word_count"] = extractedRunes
	if fullDocSizeAllowed {
		domain := r.domainAnalyzer.Analyze(input.ExtractedOutput)
		routeMeta["doc_domain"] = domain
		if domain == "resume" || domain == "invoice" {
			mode, rules := r.builder.BuildFullDocRule()
			routeMeta["matched_by"] = "doc_domain"
			return &RouterDecision{
				Matched:       true,
				RouteName:     "full_doc_model",
				TargetDocForm: string(ParentChildIndex),
				TargetMode:    mode,
				TargetRules:   rules,
				Reason:        fmt.Sprintf("matched by document domain: %s", domain),
				RouteMeta:     routeMeta,
			}, nil
		}
	}

	// 3. Profile Analysis Routing (Structural Scan)
	profile := r.profileAnalyzer.Analyze(input.ExtractedOutput)
	routeMeta["doc_profile"] = profile.String()

	if profile.IsSectionLike {
		mode, rules := r.builder.BuildSectionRule()
		routeMeta["matched_by"] = "doc_profile"
		return &RouterDecision{
			Matched:       true,
			RouteName:     "section_model",
			TargetDocForm: string(ParentChildIndex),
			TargetMode:    mode,
			TargetRules:   rules,
			Reason:        "matched by structural profile (heading density)",
			RouteMeta:     routeMeta,
		}, nil
	}

	// 4. Fallback (Let caller fallback to legacy or default)
	return &RouterDecision{
		Matched:   false,
		Reason:    fmt.Sprintf("did not match any phase-2 routing rules"),
		RouteMeta: routeMeta,
	}, nil
}

func extractedElementCount(output *dto.ExtractOutput) int {
	if output == nil {
		return 0
	}
	return len(output.Elements)
}

func extractedRuneCountUpTo(output *dto.ExtractOutput, limit int) (int, bool) {
	if output == nil {
		return 0, true
	}
	if limit < 0 {
		limit = 0
	}

	if text := strings.TrimSpace(output.Markdown); text != "" {
		return runeCountUpTo(text, limit)
	}

	count := 0
	hasContent := false
	for _, element := range output.Elements {
		content := strings.TrimSpace(element.Content)
		if content == "" {
			continue
		}
		if hasContent {
			count++
			if count > limit {
				return count, false
			}
		}
		for range content {
			count++
			if count > limit {
				return count, false
			}
		}
		hasContent = true
	}

	return count, true
}

func runeCountUpTo(text string, limit int) (int, bool) {
	count := 0
	for range text {
		count++
		if count > limit {
			return count, false
		}
	}
	return count, true
}

func normalizeDocExt(docExt string) string {
	normalized := strings.TrimSpace(strings.ToLower(docExt))
	if normalized == "" {
		return ""
	}

	if ext := filepath.Ext(normalized); ext != "" && ext != "." {
		return ext
	}

	if !strings.HasPrefix(normalized, ".") {
		normalized = "." + normalized
	}

	return normalized
}
