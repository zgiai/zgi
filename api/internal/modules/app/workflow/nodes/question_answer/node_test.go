package question_answer

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestExecuteRun_FirstEntryPausesWithoutConsumingQuery(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.UserInputs["sys.query"] = "hello"
	node := newTestQuestionAnswerNode(NodeData{
		Question:              "What is your budget?",
		AnswerType:            AnswerTypeText,
		CompletionInstruction: "Check whether the answer is enough.",
		Model:                 ModelConfig{Provider: "openai", Name: "gpt-4o-mini"},
	}, vp)

	result, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}
	if result.Status != shared.PAUSED {
		t.Fatalf("status = %s, want %s", result.Status, shared.PAUSED)
	}
	if got, _ := result.Outputs["answer"].(string); got != "" {
		t.Fatalf("answer = %q, want empty", got)
	}
	if got, _ := result.Outputs["round"].(int); got != 0 {
		t.Fatalf("round = %d, want 0", got)
	}
}

func TestExecuteRun_FirstEntryRendersQuestionTemplate(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"start", "city"}, "Beijing")
	node := newTestQuestionAnswerNode(NodeData{
		Question:   "What is the weather in {{#start.city#}}?",
		AnswerType: AnswerTypeText,
	}, vp)

	result, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}
	if result.Status != shared.PAUSED {
		t.Fatalf("status = %s, want %s", result.Status, shared.PAUSED)
	}
	if got, _ := result.Outputs["question"].(string); got != "What is the weather in Beijing?" {
		t.Fatalf("question = %q, want rendered question", got)
	}
}

func TestExecuteRun_FirstEntryFailsWhenQuestionTemplateVariableMissing(t *testing.T) {
	vp := entities.NewVariablePool()
	node := newTestQuestionAnswerNode(NodeData{
		Question:   "What is the weather in {{#start.city#}}?",
		AnswerType: AnswerTypeText,
	}, vp)

	_, err := node.executeRun(context.Background())
	if err == nil {
		t.Fatal("executeRun() error = nil, want missing variable error")
	}
	if !strings.Contains(err.Error(), "question template variable not found: start.city") {
		t.Fatalf("error = %q, want missing variable context", err.Error())
	}
}

func TestExecuteRun_ResumeUsesStoredQuestionWithoutRerendering(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"qa", "question"}, "What is the weather in Beijing?")
	vp.Add([]string{"qa", "answers"}, []AnswerRound{})
	vp.UserInputs["sys.query"] = "Sunny"
	node := newTestQuestionAnswerNode(NodeData{
		Question:   "What is the weather in {{#start.city#}}?",
		AnswerType: AnswerTypeText,
	}, vp)

	result, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}
	if result.Status != shared.SUCCEEDED {
		t.Fatalf("status = %s, want %s", result.Status, shared.SUCCEEDED)
	}
	if got, _ := result.Outputs["question"].(string); got != "What is the weather in Beijing?" {
		t.Fatalf("question = %q, want stored rendered question", got)
	}
	if got, _ := result.Outputs["answer"].(string); got != "Sunny" {
		t.Fatalf("answer = %q, want Sunny", got)
	}
}

func TestExecuteRun_ChoiceFirstEntryIncludesChoices(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.UserInputs["sys.query"] = "support"
	node := newTestQuestionAnswerNode(NodeData{
		Question:   "Choose a business type",
		AnswerType: AnswerTypeChoice,
		Choices: []Choice{
			{ID: "sales", Label: "Sales", Value: "sales"},
			{ID: "support", Label: "Support", Value: "support"},
		},
	}, vp)

	result, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}
	if result.Status != shared.PAUSED {
		t.Fatalf("status = %s, want %s", result.Status, shared.PAUSED)
	}
	choices, ok := result.Outputs["choices"].([]Choice)
	if !ok {
		t.Fatalf("choices type = %T, want []Choice", result.Outputs["choices"])
	}
	if len(choices) != 2 {
		t.Fatalf("choices length = %d, want 2", len(choices))
	}
	if got, _ := result.Outputs["answer"].(string); got != "" {
		t.Fatalf("answer = %q, want empty", got)
	}
}

func TestExecuteRun_StaticChoiceFirstEntryRendersQuestionTemplate(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"start", "kind"}, "weather")
	node := newTestQuestionAnswerNode(NodeData{
		Question:   "Choose a {{#start.kind#}} type",
		AnswerType: AnswerTypeChoice,
		Choices: []Choice{
			{ID: "sunny", Label: "Sunny", Value: "sunny"},
			{ID: "cloudy", Label: "Cloudy", Value: "cloudy"},
		},
	}, vp)

	result, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}
	if result.Status != shared.PAUSED {
		t.Fatalf("status = %s, want %s", result.Status, shared.PAUSED)
	}
	if got, _ := result.Outputs["question"].(string); got != "Choose a weather type" {
		t.Fatalf("question = %q, want rendered question", got)
	}
}

func TestExecuteRun_DynamicChoiceFirstEntryRendersQuestionTemplate(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"start", "city"}, "Beijing")
	vp.Add([]string{"upstream", "choices"}, []any{
		map[string]any{"id": "sunny", "label": "Sunny", "value": "sunny"},
		map[string]any{"id": "cloudy", "label": "Cloudy", "value": "cloudy"},
	})
	node := newTestQuestionAnswerNode(NodeData{
		Question:       "Choose weather for {{#start.city#}}",
		AnswerType:     AnswerTypeChoice,
		ChoiceMode:     ChoiceModeDynamic,
		DynamicChoices: Selector{Selector: []string{"upstream", "choices"}},
	}, vp)

	result, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}
	if result.Status != shared.PAUSED {
		t.Fatalf("status = %s, want %s", result.Status, shared.PAUSED)
	}
	if got, _ := result.Outputs["question"].(string); got != "Choose weather for Beijing" {
		t.Fatalf("question = %q, want rendered question", got)
	}
}

func TestExecuteRun_StaticChoiceResumeSelectsChoiceHandle(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"qa", "question"}, "Choose a business type")
	vp.Add([]string{"qa", "answers"}, []AnswerRound{})
	vp.UserInputs["sys.query"] = "Support"
	vp.UserInputs["question_answer_option_id"] = "support"
	node := newTestQuestionAnswerNode(NodeData{
		Question:   "Choose a business type",
		AnswerType: AnswerTypeChoice,
		Choices: []Choice{
			{ID: "sales", Label: "Sales", Value: "sales"},
			{ID: "support", Label: "Support", Value: "support"},
		},
	}, vp)

	result, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}
	if result.Status != shared.SUCCEEDED {
		t.Fatalf("status = %s, want %s", result.Status, shared.SUCCEEDED)
	}
	if result.EdgeSourceHandle != "support" {
		t.Fatalf("edge handle = %q, want support", result.EdgeSourceHandle)
	}
	if got, _ := result.Outputs["choice_id"].(string); got != "support" {
		t.Fatalf("choice_id = %q, want support", got)
	}
}

func TestExecuteRun_StaticChoiceResumeSupportsOptionIDOnly(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"qa", "question"}, "Choose a business type")
	vp.Add([]string{"qa", "answers"}, []AnswerRound{})
	vp.UserInputs["question_answer_option_id"] = "support"
	node := newTestQuestionAnswerNode(NodeData{
		Question:   "Choose a business type",
		AnswerType: AnswerTypeChoice,
		Choices: []Choice{
			{ID: "sales", Label: "Sales", Value: "sales"},
			{ID: "support", Label: "Support", Value: "support"},
		},
	}, vp)

	result, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}
	if result.Status != shared.SUCCEEDED {
		t.Fatalf("status = %s, want %s", result.Status, shared.SUCCEEDED)
	}
	if result.EdgeSourceHandle != "support" {
		t.Fatalf("edge handle = %q, want support", result.EdgeSourceHandle)
	}
	if got, _ := result.Outputs["answer"].(string); got != "support" {
		t.Fatalf("answer = %q, want support", got)
	}
}

func TestExecuteRun_ChoiceMismatchPausesBeforeMaxAnswerCount(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"qa", "question"}, "Choose a business type")
	vp.Add([]string{"qa", "answers"}, []AnswerRound{})
	vp.UserInputs["sys.query"] = "unknown"
	node := newTestQuestionAnswerNode(NodeData{
		Question:       "Choose a business type",
		AnswerType:     AnswerTypeChoice,
		MaxAnswerCount: 2,
		Choices: []Choice{
			{ID: "sales", Label: "Sales", Value: "sales"},
			{ID: "support", Label: "Support", Value: "support"},
		},
	}, vp)

	result, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}
	if result.Status != shared.PAUSED {
		t.Fatalf("status = %s, want %s", result.Status, shared.PAUSED)
	}
	if got, _ := result.Outputs["round"].(int); got != 1 {
		t.Fatalf("round = %d, want 1", got)
	}
}

func TestExecuteRun_ChoiceMismatchFailsAtMaxAnswerCount(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"qa", "question"}, "Choose a business type")
	vp.Add([]string{"qa", "answers"}, []AnswerRound{})
	vp.UserInputs["sys.query"] = "unknown"
	node := newTestQuestionAnswerNode(NodeData{
		Question:       "Choose a business type",
		AnswerType:     AnswerTypeChoice,
		MaxAnswerCount: 1,
		Choices: []Choice{
			{ID: "sales", Label: "Sales", Value: "sales"},
			{ID: "support", Label: "Support", Value: "support"},
		},
	}, vp)

	_, err := node.executeRun(context.Background())
	if err == nil {
		t.Fatal("executeRun() error = nil, want max answer count error")
	}
	if !strings.Contains(err.Error(), "after 1 rounds") {
		t.Fatalf("error = %v, want max round error", err)
	}
}

func TestExecuteRun_DynamicChoiceResumeUsesDynamicHandle(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"qa", "question"}, "Choose a plan")
	vp.Add([]string{"qa", "answers"}, []AnswerRound{})
	vp.Add([]string{"upstream", "options"}, []any{
		map[string]any{"id": "vip", "label": "VIP", "value": "vip"},
	})
	vp.UserInputs["sys.query"] = "VIP"
	node := newTestQuestionAnswerNode(NodeData{
		Question:       "Choose a plan",
		AnswerType:     AnswerTypeChoice,
		ChoiceMode:     ChoiceModeDynamic,
		DynamicChoices: Selector{Selector: []string{"upstream", "options"}},
	}, vp)

	result, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}
	if result.Status != shared.SUCCEEDED {
		t.Fatalf("status = %s, want %s", result.Status, shared.SUCCEEDED)
	}
	if result.EdgeSourceHandle != DynamicChoiceHandle {
		t.Fatalf("edge handle = %q, want %s", result.EdgeSourceHandle, DynamicChoiceHandle)
	}
	if got, _ := result.Outputs["choice_id"].(string); got != "vip" {
		t.Fatalf("choice_id = %q, want vip", got)
	}
}

func TestExecuteRun_DynamicChoiceFirstEntryPausesWithResumeInputs(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"upstream", "options"}, []any{
		map[string]any{"id": "vip", "label": "VIP", "value": "vip"},
	})
	vp.UserInputs["sys.query"] = "VIP"
	vp.UserInputs["question_answer_option_id"] = "vip"
	node := newTestQuestionAnswerNode(NodeData{
		Question:       "Choose a plan",
		AnswerType:     AnswerTypeChoice,
		ChoiceMode:     ChoiceModeDynamic,
		DynamicChoices: Selector{Selector: []string{"upstream", "options"}},
	}, vp)

	result, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}
	if result.Status != shared.PAUSED {
		t.Fatalf("status = %s, want %s", result.Status, shared.PAUSED)
	}
	if got, _ := result.Outputs["question"].(string); got != "Choose a plan" {
		t.Fatalf("question = %q, want Choose a plan", got)
	}
	if got, _ := result.Outputs["answer"].(string); got != "" {
		t.Fatalf("answer = %q, want empty", got)
	}
}

func TestParseDynamicChoices_AcceptsQuestionAnswerChoiceOutput(t *testing.T) {
	choices, err := parseDynamicChoices([]Choice{
		{ID: "A", Label: "Option A", Value: "A"},
		{ID: "B", Label: "Option B", Value: "B"},
	})
	if err != nil {
		t.Fatalf("parseDynamicChoices() error = %v", err)
	}
	if len(choices) != 2 {
		t.Fatalf("choices length = %d, want 2", len(choices))
	}
	if choices[1].ID != "B" || choices[1].Label != "Option B" || choices[1].Value != "B" {
		t.Fatalf("choice = %#v, want normalized B option", choices[1])
	}
}

func TestExecuteRun_TextResumeWithoutExtractionSucceedsWithoutLLM(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"qa", "question"}, "What city do you need?")
	vp.Add([]string{"qa", "answers"}, []AnswerRound{})
	vp.UserInputs["sys.query"] = "puppy"
	llm := &mockQuestionAnswerLLMClient{
		appChat: func(context.Context, *llmclient.AppContext, *llmadapter.ChatRequest) (*llmadapter.ChatResponse, error) {
			t.Fatal("AppChat should not be called when answer extraction is disabled")
			return nil, nil
		},
	}
	node := newTestQuestionAnswerNodeWithClient(NodeData{
		Question:   "What city do you need?",
		AnswerType: AnswerTypeText,
	}, vp, llm)

	result, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}
	if result.Status != shared.SUCCEEDED {
		t.Fatalf("status = %s, want %s", result.Status, shared.SUCCEEDED)
	}
	if got, _ := result.Outputs["answer"].(string); got != "puppy" {
		t.Fatalf("answer = %q, want puppy", got)
	}
}

func TestValidateConfig_TextExtractionRequiresModel(t *testing.T) {
	node := newTestQuestionAnswerNode(NodeData{
		Question:          "What city do you need?",
		AnswerType:        AnswerTypeText,
		ExtractFromAnswer: true,
		ExtractionFields: []ExtractionField{
			{Name: "city", Type: ExtractionFieldTypeString, Required: true},
		},
	}, entities.NewVariablePool())

	err := node.validateConfig()
	if err == nil {
		t.Fatal("validateConfig() error = nil, want model config error")
	}
	if !strings.Contains(err.Error(), "requires model config") {
		t.Fatalf("error = %v, want model config error", err)
	}
}

func TestExecuteRun_TextExtractionSucceedsWhenRequiredFieldPresent(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"qa", "question"}, "What city do you need?")
	vp.Add([]string{"qa", "answers"}, []AnswerRound{})
	vp.UserInputs["sys.query"] = "Beijing"
	node := newTestQuestionAnswerNodeWithClient(NodeData{
		Question:          "What city do you need?",
		AnswerType:        AnswerTypeText,
		ExtractFromAnswer: true,
		ExtractionFields: []ExtractionField{
			{Name: "city", Type: ExtractionFieldTypeString, Required: true},
		},
		Model: ModelConfig{Provider: "openai", Name: "gpt-4o-mini"},
	}, vp, mockExtractionClient(`{"fields":{"city":"Beijing"},"missing_fields":[],"follow_up_question":"","reason":"ok"}`))

	result, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}
	if result.Status != shared.SUCCEEDED {
		t.Fatalf("status = %s, want %s", result.Status, shared.SUCCEEDED)
	}
	fields, ok := result.Outputs["extracted_fields"].(map[string]any)
	if !ok {
		t.Fatalf("extracted_fields type = %T, want map[string]any", result.Outputs["extracted_fields"])
	}
	if got := fields["city"]; got != "Beijing" {
		t.Fatalf("extracted city = %v, want Beijing", got)
	}
	if got := result.Outputs["city"]; got != "Beijing" {
		t.Fatalf("city output = %v, want Beijing", got)
	}
}

func TestExecuteRun_TextExtractionMissingRequiredFieldPausesBeforeMax(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"qa", "question"}, "What city do you need?")
	vp.Add([]string{"qa", "answers"}, []AnswerRound{})
	vp.UserInputs["sys.query"] = "puppy"
	node := newTestQuestionAnswerNodeWithClient(NodeData{
		Question:          "What city do you need?",
		AnswerType:        AnswerTypeText,
		ExtractFromAnswer: true,
		MaxAnswerCount:    2,
		ExtractionFields: []ExtractionField{
			{Name: "city", Type: ExtractionFieldTypeString, Required: true},
		},
		Model: ModelConfig{Provider: "openai", Name: "gpt-4o-mini"},
	}, vp, mockExtractionClient(`{"fields":{},"missing_fields":["city"],"follow_up_question":"Which city do you need?","reason":"missing city"}`))

	result, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}
	if result.Status != shared.PAUSED {
		t.Fatalf("status = %s, want %s", result.Status, shared.PAUSED)
	}
	if got, _ := result.Outputs["question"].(string); got != "Which city do you need?" {
		t.Fatalf("follow-up question = %q, want Which city do you need?", got)
	}
}

func TestExecuteRun_TextExtractionMissingRequiredFieldFailsAtMax(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"qa", "question"}, "What city do you need?")
	vp.Add([]string{"qa", "answers"}, []AnswerRound{})
	vp.UserInputs["sys.query"] = "puppy"
	node := newTestQuestionAnswerNodeWithClient(NodeData{
		Question:          "What city do you need?",
		AnswerType:        AnswerTypeText,
		ExtractFromAnswer: true,
		MaxAnswerCount:    1,
		ExtractionFields: []ExtractionField{
			{Name: "city", Type: ExtractionFieldTypeString, Required: true},
		},
		Model: ModelConfig{Provider: "openai", Name: "gpt-4o-mini"},
	}, vp, mockExtractionClient(`{"fields":{},"missing_fields":["city"],"follow_up_question":"Which city do you need?","reason":"missing city"}`))

	_, err := node.executeRun(context.Background())
	if err == nil {
		t.Fatal("executeRun() error = nil, want missing field error")
	}
	if !strings.Contains(err.Error(), "missing required extracted fields: city") {
		t.Fatalf("error = %v, want missing city error", err)
	}
}

func TestExecuteRun_TextExtractionRejectsWrongFieldType(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"qa", "question"}, "How many seats?")
	vp.Add([]string{"qa", "answers"}, []AnswerRound{})
	vp.UserInputs["sys.query"] = "two"
	node := newTestQuestionAnswerNodeWithClient(NodeData{
		Question:          "How many seats?",
		AnswerType:        AnswerTypeText,
		ExtractFromAnswer: true,
		ExtractionFields: []ExtractionField{
			{Name: "seats", Type: ExtractionFieldTypeNumber, Required: true},
		},
		Model: ModelConfig{Provider: "openai", Name: "gpt-4o-mini"},
	}, vp, mockExtractionClient(`{"fields":{"seats":"two"},"missing_fields":[],"follow_up_question":"","reason":"ok"}`))

	_, err := node.executeRun(context.Background())
	if err == nil {
		t.Fatal("executeRun() error = nil, want field type error")
	}
	if !strings.Contains(err.Error(), "extracted field seats must be number") {
		t.Fatalf("error = %v, want field type error", err)
	}
}

func TestTextExtractionSystemPrompt_RequiresJSONFields(t *testing.T) {
	prompt := textExtractionSystemPrompt()
	for _, want := range []string{"extract fields", "missing_fields", "follow_up_question"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("system prompt missing %q: %s", want, prompt)
		}
	}
}

func TestTextExtractionUserPrompt_IncludesConfiguredFields(t *testing.T) {
	prompt := textExtractionUserPrompt("Extract city.", "What city?", "Beijing", nil, []ExtractionField{
		{Name: "city", Type: ExtractionFieldTypeString, Required: true, Description: "city name"},
	})
	for _, want := range []string{"extraction_instruction", "fields", "city"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("user prompt missing %q: %s", want, prompt)
		}
	}
}

func newTestQuestionAnswerNode(data NodeData, vp *entities.VariablePool) *Node {
	return newTestQuestionAnswerNodeWithClient(data, vp, nil)
}

func newTestQuestionAnswerNodeWithClient(data NodeData, vp *entities.VariablePool, llm llmclient.LLMClient) *Node {
	normalizeNodeData(&data)
	return &Node{
		NodeStruct: base.NodeStruct{
			NodeID:            "qa",
			NodeType:          shared.QuestionAnswer,
			GraphRuntimeState: entities.NewGraphRuntimeState(vp),
		},
		NodeData:  data,
		llmClient: llm,
	}
}

func mockExtractionClient(content string) *mockQuestionAnswerLLMClient {
	return &mockQuestionAnswerLLMClient{
		appChat: func(context.Context, *llmclient.AppContext, *llmadapter.ChatRequest) (*llmadapter.ChatResponse, error) {
			return &llmadapter.ChatResponse{
				Choices: []llmadapter.Choice{
					{
						Message: llmadapter.Message{
							Content: content,
						},
					},
				},
			}, nil
		},
	}
}

type mockQuestionAnswerLLMClient struct {
	appChat func(context.Context, *llmclient.AppContext, *llmadapter.ChatRequest) (*llmadapter.ChatResponse, error)
}

func (m *mockQuestionAnswerLLMClient) Chat(context.Context, string, *llmadapter.ChatRequest) (*llmadapter.ChatResponse, error) {
	return nil, fmt.Errorf("unexpected Chat call")
}

func (m *mockQuestionAnswerLLMClient) ChatStream(context.Context, string, *llmadapter.ChatRequest) (<-chan llmadapter.StreamResponse, error) {
	return nil, fmt.Errorf("unexpected ChatStream call")
}

func (m *mockQuestionAnswerLLMClient) CreateResponse(context.Context, string, *llmadapter.CreateResponseRequest) (*llmadapter.CreateResponseResponse, error) {
	return nil, fmt.Errorf("unexpected CreateResponse call")
}

func (m *mockQuestionAnswerLLMClient) Embed(context.Context, string, *llmadapter.EmbeddingsRequest) (*llmadapter.EmbeddingsResponse, error) {
	return nil, fmt.Errorf("unexpected Embed call")
}

func (m *mockQuestionAnswerLLMClient) CreateImage(context.Context, string, *llmadapter.ImageRequest) (*llmadapter.ImageResponse, error) {
	return nil, fmt.Errorf("unexpected CreateImage call")
}

func (m *mockQuestionAnswerLLMClient) Rerank(context.Context, string, *llmadapter.RerankRequest) (*llmadapter.RerankResponse, error) {
	return nil, fmt.Errorf("unexpected Rerank call")
}

func (m *mockQuestionAnswerLLMClient) AppChat(ctx context.Context, appCtx *llmclient.AppContext, req *llmadapter.ChatRequest) (*llmadapter.ChatResponse, error) {
	if m.appChat == nil {
		return nil, fmt.Errorf("unexpected AppChat call")
	}
	return m.appChat(ctx, appCtx, req)
}

func (m *mockQuestionAnswerLLMClient) AppChatStream(context.Context, *llmclient.AppContext, *llmadapter.ChatRequest) (<-chan llmadapter.StreamResponse, error) {
	return nil, fmt.Errorf("unexpected AppChatStream call")
}

func (m *mockQuestionAnswerLLMClient) AppCreateResponse(context.Context, *llmclient.AppContext, *llmadapter.CreateResponseRequest) (*llmadapter.CreateResponseResponse, error) {
	return nil, fmt.Errorf("unexpected AppCreateResponse call")
}

func (m *mockQuestionAnswerLLMClient) AppEmbed(context.Context, *llmclient.AppContext, *llmadapter.EmbeddingsRequest) (*llmadapter.EmbeddingsResponse, error) {
	return nil, fmt.Errorf("unexpected AppEmbed call")
}

func (m *mockQuestionAnswerLLMClient) AppCreateImage(context.Context, *llmclient.AppContext, *llmadapter.ImageRequest) (*llmadapter.ImageResponse, error) {
	return nil, fmt.Errorf("unexpected AppCreateImage call")
}

func (m *mockQuestionAnswerLLMClient) AppRerank(context.Context, *llmclient.AppContext, *llmadapter.RerankRequest) (*llmadapter.RerankResponse, error) {
	return nil, fmt.Errorf("unexpected AppRerank call")
}
