package suggestedquestions

import "testing"

func TestBuildContextExtractsWorkflowSignals(t *testing.T) {
	graph := map[string]interface{}{
		"nodes": []interface{}{
			map[string]interface{}{
				"id": "start",
				"data": map[string]interface{}{
					"type": "start",
					"variables": []interface{}{
						map[string]interface{}{
							"variable":    "customer_request",
							"type":        "paragraph",
							"description": "Customer request submitted by the user",
							"required":    true,
						},
						map[string]interface{}{
							"variable": "api_key",
							"type":     "secret",
						},
					},
				},
			},
			map[string]interface{}{
				"id": "llm",
				"data": map[string]interface{}{
					"type":  "llm",
					"title": "Classify request",
					"model": map[string]interface{}{
						"provider": "qwen",
						"name":     "qwen3.6-plus",
					},
					"prompt_template": []interface{}{
						map[string]interface{}{
							"role": "user",
							"text": "Classify the customer request and recommend the next action.",
						},
					},
				},
			},
			map[string]interface{}{
				"id": "note",
				"data": map[string]interface{}{
					"type":  "note",
					"title": "Run guide",
					"text":  "Enter a service request to test the workflow.",
				},
			},
		},
	}

	ctx := BuildContext(BuildContextInput{
		Locale:           "zh-CN",
		AgentName:        "Service request router",
		AgentDescription: "Classifies user requests by intent",
		WorkflowType:     "advanced-chat",
		Graph:            graph,
		Features: map[string]interface{}{
			"suggested_questions": []interface{}{"When will my order ship?"},
		},
	})

	if ctx.Locale != "zh-Hans" {
		t.Fatalf("Locale = %q, want zh-Hans", ctx.Locale)
	}
	if len(ctx.StartVariables) != 1 || ctx.StartVariables[0].Name != "customer_request" {
		t.Fatalf("StartVariables = %#v", ctx.StartVariables)
	}
	if len(ctx.LLMPrompts) != 1 || ctx.LLMPrompts[0].Model != "qwen/qwen3.6-plus" {
		t.Fatalf("LLMPrompts = %#v", ctx.LLMPrompts)
	}
	if len(ctx.Notes) != 1 || ctx.Notes[0].Title != "Run guide" {
		t.Fatalf("Notes = %#v", ctx.Notes)
	}
	if len(ctx.ExistingQuestions) != 1 {
		t.Fatalf("ExistingQuestions = %#v", ctx.ExistingQuestions)
	}
}

func TestBuildContextKeepsLegacyOpeningStatement(t *testing.T) {
	ctx := BuildContext(BuildContextInput{
		Features: map[string]interface{}{
			"opening_statement": "Welcome to the enterprise assistant. Please enter your question.",
		},
	})

	if ctx.OpeningStatement != "Welcome to the enterprise assistant. Please enter your question." {
		t.Fatalf("OpeningStatement = %q", ctx.OpeningStatement)
	}
}

func TestBuildContextExtractsCompletionPromptTemplateObject(t *testing.T) {
	ctx := BuildContext(BuildContextInput{
		Graph: map[string]interface{}{
			"nodes": []interface{}{
				map[string]interface{}{
					"id": "llm",
					"data": map[string]interface{}{
						"type":  "llm",
						"title": "Generate reply",
						"model": map[string]interface{}{
							"provider": "openai",
							"name":     "gpt-4o",
						},
						"prompt_template": map[string]interface{}{
							"text": "Generate a structured reply based on the user request.",
						},
					},
				},
			},
		},
	})

	if len(ctx.LLMPrompts) != 1 {
		t.Fatalf("LLMPrompts = %#v", ctx.LLMPrompts)
	}
	if ctx.LLMPrompts[0].Text != "Generate a structured reply based on the user request." {
		t.Fatalf("LLMPrompts[0].Text = %q", ctx.LLMPrompts[0].Text)
	}
	if ctx.LLMPrompts[0].Role != "user" {
		t.Fatalf("LLMPrompts[0].Role = %q", ctx.LLMPrompts[0].Role)
	}
}

func TestBuildContextAddsDependencyLabelsForKebabCaseNodeTypes(t *testing.T) {
	ctx := BuildContext(BuildContextInput{
		Graph: map[string]interface{}{
			"nodes": []interface{}{
				map[string]interface{}{"id": "knowledge", "data": map[string]interface{}{"type": "knowledge-retrieval", "title": "Knowledge base"}},
				map[string]interface{}{"id": "db", "data": map[string]interface{}{"type": "call-database", "title": "Database lookup"}},
				map[string]interface{}{"id": "sql", "data": map[string]interface{}{"type": "sql-generator", "title": "Generate SQL"}},
				map[string]interface{}{"id": "sms", "data": map[string]interface{}{"type": "notification-sms", "title": "SMS notification"}},
				map[string]interface{}{"id": "image", "data": map[string]interface{}{"type": "image-gen", "title": "Generate image"}},
				map[string]interface{}{"id": "doc", "data": map[string]interface{}{"type": "document-extractor", "title": "Extract document"}},
				map[string]interface{}{"id": "http", "data": map[string]interface{}{"type": "http-request", "title": "Call API"}},
			},
		},
	})

	dependencies := map[string]string{}
	for _, capability := range ctx.Capabilities {
		dependencies[capability.Type] = capability.Dependency
	}

	for nodeType, want := range map[string]string{
		"knowledge-retrieval": "knowledge_base",
		"call-database":       "database",
		"sql-generator":       "database",
		"notification-sms":    "sms_channel",
		"image-gen":           "image_model",
		"document-extractor":  "file_input",
		"http-request":        "http_api",
	} {
		if dependencies[nodeType] != want {
			t.Fatalf("dependency for %s = %q, want %q; all = %#v", nodeType, dependencies[nodeType], want, dependencies)
		}
	}
}
