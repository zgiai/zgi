package service

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	promptdto "github.com/zgiai/zgi/api/internal/modules/prompts/dto"
	promptmodel "github.com/zgiai/zgi/api/internal/modules/prompts/model"
)

func TestRuntimeReferenceLabelDefaultsToLatest(t *testing.T) {
	if got := runtimeReferenceLabel(RuntimePromptReference{}); got != latestLabel {
		t.Fatalf("expected default runtime label %q, got %q", latestLabel, got)
	}
}

func TestRuntimeReferenceLabelUsesExplicitTrimmedLabel(t *testing.T) {
	label := " production "
	if got := runtimeReferenceLabel(RuntimePromptReference{Label: &label}); got != "production" {
		t.Fatalf("expected explicit runtime label %q, got %q", "production", got)
	}
}

func TestVersionsByLabelKeepsHighestMatchingVersionPerPrompt(t *testing.T) {
	versions := []*promptmodel.PromptVersion{
		{PromptID: "prompt-a", Version: 1, Labels: []string{productionLabel}},
		{PromptID: "prompt-a", Version: 3, Labels: []string{productionLabel}},
		{PromptID: "prompt-b", Version: 2, Labels: []string{latestLabel}},
	}

	got := versionsByLabel(versions, productionLabel)

	if got["prompt-a"] == nil || got["prompt-a"].Version != 3 {
		t.Fatalf("expected prompt-a production version 3, got %#v", got["prompt-a"])
	}
	if got["prompt-b"] != nil {
		t.Fatalf("expected prompt-b to have no production version, got %#v", got["prompt-b"])
	}
}

func TestFirstVersionWithLabelReturnsFirstMatchingVersion(t *testing.T) {
	versions := []*promptmodel.PromptVersion{
		{Version: 4, Labels: []string{latestLabel}},
		{Version: 2, Labels: []string{productionLabel}},
	}

	got := firstVersionWithLabel(versions, productionLabel)
	if got == nil || got.Version != 2 {
		t.Fatalf("expected production version 2, got %#v", got)
	}
}

func TestUniqueLabelsKeepsLatestSeparateFromProduction(t *testing.T) {
	gotLatest := uniqueLabels([]string{productionLabel, latestLabel, " production ", ""}, true)
	if want := []string{productionLabel, latestLabel}; !reflect.DeepEqual(gotLatest, want) {
		t.Fatalf("unique labels with latest = %#v, want %#v", gotLatest, want)
	}

	gotHistorical := uniqueLabels([]string{productionLabel, latestLabel}, false)
	if want := []string{productionLabel}; !reflect.DeepEqual(gotHistorical, want) {
		t.Fatalf("unique labels without latest = %#v, want %#v", gotHistorical, want)
	}
}

func TestReassignedVersionLabelsPublishingHistoricalVersionPreservesLatest(t *testing.T) {
	targetSet := map[string]struct{}{productionLabel: {}}
	latestVersion := &promptmodel.PromptVersion{
		Version: 4,
		Labels:  []string{latestLabel},
	}
	historicalVersion := &promptmodel.PromptVersion{
		Version: 2,
		Labels:  []string{},
	}

	gotLatest := reassignedVersionLabels(latestVersion, 2, []string{productionLabel}, targetSet)
	if want := []string{latestLabel}; !reflect.DeepEqual(gotLatest, want) {
		t.Fatalf("latest version labels = %#v, want %#v", gotLatest, want)
	}

	gotHistorical := reassignedVersionLabels(historicalVersion, 2, []string{productionLabel}, targetSet)
	if want := []string{productionLabel}; !reflect.DeepEqual(gotHistorical, want) {
		t.Fatalf("historical version labels = %#v, want %#v", gotHistorical, want)
	}
}

func TestReassignedVersionLabelsPublishingLatestVersionKeepsLatestAndProduction(t *testing.T) {
	targetLabels := []string{productionLabel, latestLabel}
	targetSet := map[string]struct{}{productionLabel: {}, latestLabel: {}}
	latestVersion := &promptmodel.PromptVersion{
		Version: 4,
		Labels:  []string{latestLabel},
	}
	previousProductionVersion := &promptmodel.PromptVersion{
		Version: 2,
		Labels:  []string{productionLabel},
	}

	gotLatest := reassignedVersionLabels(latestVersion, 4, targetLabels, targetSet)
	if want := []string{productionLabel, latestLabel}; !reflect.DeepEqual(gotLatest, want) {
		t.Fatalf("latest production labels = %#v, want %#v", gotLatest, want)
	}

	gotPrevious := reassignedVersionLabels(previousProductionVersion, 4, targetLabels, targetSet)
	if want := []string{}; !reflect.DeepEqual(gotPrevious, want) {
		t.Fatalf("previous production labels = %#v, want %#v", gotPrevious, want)
	}
}

func TestReassignedVersionLabelsCreatingNewVersionMovesLatest(t *testing.T) {
	targetSet := map[string]struct{}{latestLabel: {}}
	previousLatest := &promptmodel.PromptVersion{
		Version: 3,
		Labels:  []string{productionLabel, latestLabel},
	}

	got := reassignedVersionLabels(previousLatest, 4, []string{latestLabel}, targetSet)
	if want := []string{productionLabel}; !reflect.DeepEqual(got, want) {
		t.Fatalf("previous latest labels = %#v, want %#v", got, want)
	}
}

func TestReassignLabelsRequiresExistingTargetWhenRequested(t *testing.T) {
	err := reassignLabels(nil, []*promptmodel.PromptVersion{
		{Version: 1, Labels: []string{latestLabel}},
	}, 2, []string{"stable"}, true)
	if err == nil {
		t.Fatal("expected missing target version error")
	}
	if !strings.Contains(err.Error(), "prompt version not found") {
		t.Fatalf("expected prompt version not found error, got %v", err)
	}
}

func TestPromptOptimizerOutputLanguageNormalizesInterfaceLocale(t *testing.T) {
	tests := []struct {
		name     string
		language string
		want     string
	}{
		{name: "simplified chinese", language: "zh-Hans", want: "Simplified Chinese"},
		{name: "english", language: "en-US", want: "English"},
		{name: "fallback custom", language: "fr-FR", want: "fr-FR"},
		{name: "empty", language: " ", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := promptOptimizerOutputLanguage(tt.language); got != tt.want {
				t.Fatalf("expected language %q, got %q", tt.want, got)
			}
		})
	}
}

func TestBuildPromptOptimizerUserPromptUsesInterfaceLanguageForDeepOptimization(t *testing.T) {
	prompt := buildPromptOptimizerUserPrompt(
		promptOptimizerGoalDeep,
		"Write a sales email.",
		true,
		nil,
		"Simplified Chinese",
		"",
		0,
	)

	if !strings.Contains(prompt, "Use the system/interface language for the final optimized prompt: Simplified Chinese.") {
		t.Fatalf("expected deep optimizer prompt to require interface language, got:\n%s", prompt)
	}
	if strings.Contains(prompt, "Match the primary language of the user's original prompt") {
		t.Fatalf("deep optimizer prompt should not prefer original prompt language, got:\n%s", prompt)
	}
}

func TestBuildPromptOptimizerUserPromptIncludesLengthTargetAndEditInstruction(t *testing.T) {
	prompt := buildPromptOptimizerUserPrompt(
		promptOptimizerGoalGeneral,
		"Original prompt.",
		true,
		[]string{"{{user_name}}"},
		"Simplified Chinese",
		"只增强工具调用规则，保持原结构。",
		16000,
	)

	for _, want := range []string{
		"Final prompt must be no more than 16000 characters.",
		"只增强工具调用规则，保持原结构。",
		"Follow this edit request unless it conflicts with variable preservation",
		"{{user_name}}",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("optimizer prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestPromptOptimizerMaxTokensUsesModelLimitAndFallbacks(t *testing.T) {
	tests := []struct {
		name   string
		model  *llmdefaultservice.ResolvedModel
		goal   string
		expect int
	}{
		{name: "regular fallback", model: &llmdefaultservice.ResolvedModel{}, goal: promptOptimizerGoalGeneral, expect: promptOptimizerDefaultMaxTokens},
		{name: "deep fallback", model: &llmdefaultservice.ResolvedModel{}, goal: promptOptimizerGoalDeep, expect: promptOptimizerDeepDefaultMaxTokens},
		{name: "model limit", model: &llmdefaultservice.ResolvedModel{MaxOutputTokens: 4096}, goal: promptOptimizerGoalDeep, expect: 4096},
		{name: "platform cap", model: &llmdefaultservice.ResolvedModel{MaxOutputTokens: 64000}, goal: promptOptimizerGoalGeneral, expect: promptOptimizerPlatformMaxTokens},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := promptOptimizerMaxTokens(tt.model, tt.goal); got != tt.expect {
				t.Fatalf("max tokens = %d, want %d", got, tt.expect)
			}
		})
	}
}

func TestPromptOptimizerFinishReasonIsTruncated(t *testing.T) {
	tests := []struct {
		reason string
		want   bool
	}{
		{reason: "length", want: true},
		{reason: "max_tokens", want: true},
		{reason: "token_limit_reached", want: true},
		{reason: "stop", want: false},
		{reason: "", want: false},
	}

	for _, tt := range tests {
		if got := promptOptimizerFinishReasonIsTruncated(tt.reason); got != tt.want {
			t.Fatalf("finish reason %q truncated = %v, want %v", tt.reason, got, tt.want)
		}
	}
}

func TestNormalizePromptOptimizerInputRemovesZGISlotWrappers(t *testing.T) {
	raw := `你叫<zgi:slot name="agent_name" placeholder="智能体名称">小雪</zgi:slot>，请使用<zgi:knowledge id="ds-1">产品知识库</zgi:knowledge>。`
	got := normalizePromptOptimizerInput(raw)

	if strings.Contains(got, "<zgi:slot") {
		t.Fatalf("expected slot wrapper to be removed, got:\n%s", got)
	}
	if !strings.Contains(got, "你叫小雪") {
		t.Fatalf("expected slot value to be preserved as plain text, got:\n%s", got)
	}
	if !strings.Contains(got, `<zgi:knowledge id="ds-1">产品知识库</zgi:knowledge>`) {
		t.Fatalf("expected knowledge variable to be preserved, got:\n%s", got)
	}
}

func TestDetectPromptOptimizerVariablesIncludesZGICapabilityBlocks(t *testing.T) {
	raw := `当问题相关时使用<zgi:knowledge id="ds-1">产品知识库</zgi:knowledge>，计算时使用<zgi:skill id="calculator">计算器</zgi:skill>。
读取<zgi:database id="db-1">客户库</zgi:database>和<zgi:table id="table-1">订单表</zgi:table>，必要时调用<zgi:workflow id="wf-1">售后流程</zgi:workflow>。`
	variables := detectPromptOptimizerVariables(raw)
	joined := strings.Join(variables, "\n")

	for _, want := range []string{
		`<zgi:knowledge id="ds-1">产品知识库</zgi:knowledge>`,
		`<zgi:skill id="calculator">计算器</zgi:skill>`,
		`<zgi:database id="db-1">客户库</zgi:database>`,
		`<zgi:table id="table-1">订单表</zgi:table>`,
		`<zgi:workflow id="wf-1">售后流程</zgi:workflow>`,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected variable %q to be detected, got %#v", want, variables)
		}
	}
}

func TestNormalizeVersionInputRejectsInvalidChatMessages(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "invalid role",
			content: `[{"role":"tool","content":"Call calculator"}]`,
			want:    "must use role system, user, or assistant",
		},
		{
			name:    "empty content",
			content: `[{"role":"user","content":"  "}]`,
			want:    "content cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, _, err := normalizeVersionInput(promptdto.PromptVersionInput{
				PromptType: string(promptmodel.PromptTypeChat),
				Content:    json.RawMessage(tt.content),
			}, true)
			if err == nil {
				t.Fatalf("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestRenderPromptPlaygroundPromptReplacesDollarInputAndPreservesZGIBlocks(t *testing.T) {
	raw := `Review ${input} and keep <zgi:workflow id="wf-1">Workflow</zgi:workflow>.`
	rendered, detected := renderPromptPlaygroundPrompt(raw, "customer complaint", nil)

	if !strings.Contains(rendered, "Review customer complaint") {
		t.Fatalf("expected ${input} to be replaced, got %q", rendered)
	}
	if !strings.Contains(rendered, `<zgi:workflow id="wf-1">Workflow</zgi:workflow>`) {
		t.Fatalf("expected ZGI workflow block to be preserved, got %q", rendered)
	}
	joined := strings.Join(detected, "\n")
	if !strings.Contains(joined, "${input}") {
		t.Fatalf("expected ${input} to be detected, got %#v", detected)
	}
	if !strings.Contains(joined, `<zgi:workflow id="wf-1">Workflow</zgi:workflow>`) {
		t.Fatalf("expected ZGI workflow block to be detected, got %#v", detected)
	}
}
