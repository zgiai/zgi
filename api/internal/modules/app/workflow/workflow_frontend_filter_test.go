package workflow

import (
	"reflect"
	"testing"
)

type dummyFile struct {
	ID       string
	Filename string
}

func TestFilterFrontendInputs(t *testing.T) {
	tests := []struct {
		name     string
		nodeType string
		inputs   map[string]interface{}
		want     map[string]interface{}
	}{
		{
			name:     "remove internal keys",
			nodeType: "code",
			inputs: map[string]interface{}{
				"a":                      1,
				"__edge_source_handle__": "xxx",
			},
			want: map[string]interface{}{
				"a": 1,
			},
		},
		{
			name:     "remove system blacklist",
			nodeType: "unknown-node",
			inputs: map[string]interface{}{
				"user_var":            "hello",
				"sys.user_id":         "123",
				"conversation_params": map[string]interface{}{"source": "api"},
				"metadata":            map[string]interface{}{"foo": "bar"},
			},
			want: map[string]interface{}{
				"user_var": "hello",
			},
		},
		{
			name:     "LLM node keep specific sys keys",
			nodeType: "llm",
			inputs: map[string]interface{}{
				"sys.query":           "what is ai?",
				"sys.conversation_id": "conv1",
				"sys.user_id":         "user123",
				"sys.dialogue_count":  5,
				"other_var":           "val",
			},
			want: map[string]interface{}{
				"sys.query":           "what is ai?",
				"sys.conversation_id": "conv1",
				"sys.user_id":         "user123",
				"sys.dialogue_count":  5,
				"other_var":           "val",
			},
		},
		{
			name:     "Start node no longer keeps sys keys",
			nodeType: "start",
			inputs: map[string]interface{}{
				"sys.query":           "hello",
				"sys.conversation_id": "c1",
				"sys.user_id":         "u1",
				"user_input":          "data",
			},
			want: map[string]interface{}{
				"user_input": "data",
			},
		},
		{
			name:     "Conversation history summary",
			nodeType: "llm",
			inputs: map[string]interface{}{
				"sys.conversation_history": []interface{}{
					map[string]interface{}{"role": "user", "content": "hi"},
					map[string]interface{}{"role": "assistant", "content": "hello"},
				},
			},
			want: map[string]interface{}{
				"sys.conversation_history": []interface{}{
					map[string]interface{}{"role": "user", "content": "hi"},
					map[string]interface{}{"role": "assistant", "content": "hello"},
				},
			},
		},
		{
			name:     "Document extractor: only keep file inputs",
			nodeType: "document-extractor",
			inputs: map[string]interface{}{
				"log_file": map[string]interface{}{
					"id":       "file-1",
					"filename": "test.pdf",
				},
				"other_var": "noise",
				"sys.query": "query",
			},
			want: map[string]interface{}{
				"log_file": map[string]interface{}{
					"id":       "file-1",
					"filename": "test.pdf",
				},
			},
		},
		{
			name:     "Document extractor: keep file structs",
			nodeType: "document-extractor",
			inputs: map[string]interface{}{
				"file_obj": &dummyFile{ID: "f1", Filename: "f1.txt"},
				"str":      "text",
			},
			want: map[string]interface{}{
				"file_obj": &dummyFile{ID: "f1", Filename: "f1.txt"},
			},
		},
		{
			name:     "Parameter Extractor: configured sys.* variables pass through",
			nodeType: "parameter-extractor",
			inputs: map[string]interface{}{
				"query":     "extract the name and age",
				"files":     []interface{}{},
				"sys.query": "what is ai?",
				"other_var": "noise",
			},
			want: map[string]interface{}{
				"query":     "extract the name and age",
				"files":     []interface{}{},
				"sys.query": "what is ai?",
				"other_var": "noise",
			},
		},
		{
			name:     "Parameter Extractor: configured system query key passes through",
			nodeType: "parameter-extractor",
			inputs: map[string]interface{}{
				"input":     "extracted text",
				"sys.query": "what is ai?",
			},
			want: map[string]interface{}{
				"input":     "extracted text",
				"sys.query": "what is ai?",
			},
		},
		{
			name:     "Knowledge Retrieval: configured sys.* variables pass through",
			nodeType: "knowledge-retrieval",
			inputs: map[string]interface{}{
				"query":     "search keyword",
				"sys.query": "original input",
				"other_var": "noise",
			},
			want: map[string]interface{}{
				"query":     "search keyword",
				"sys.query": "original input",
				"other_var": "noise",
			},
		},
		{
			name:     "Knowledge Retrieval: empty inputs returns empty",
			nodeType: "knowledge-retrieval",
			inputs:   map[string]interface{}{},
			want:     map[string]interface{}{},
		},
		{
			name:     "HTTP request: only keep request fields",
			nodeType: "http-request",
			inputs: map[string]interface{}{
				"url":          "http://baidu.com",
				"method":       "GET",
				"header":       map[string]interface{}{},
				"param":        map[string]interface{}{},
				"body":         nil,
				"auth":         nil,
				"retry_config": map[string]interface{}{"max_times": 3},
				"sys.user_id":  "u1",
			},
			want: map[string]interface{}{
				"url":    "http://baidu.com",
				"method": "GET",
				"header": map[string]interface{}{},
				"param":  map[string]interface{}{},
				"auth":   nil,
			},
		},
		{
			name:     "Call database: keep only sql",
			nodeType: "call-database",
			inputs: map[string]interface{}{
				"data_source":     map[string]interface{}{"id": "ds-1"},
				"schema_tables":   []interface{}{"main.users"},
				"table_selection": []interface{}{map[string]interface{}{"name": "users"}},
				"sql":             "SELECT 1",
				"execution":       map[string]interface{}{"timeout_seconds": 30},
			},
			want: map[string]interface{}{
				"sql": "SELECT 1",
			},
		},
		{
			name:     "SQL generator: keep only prompt",
			nodeType: "sql-generator",
			inputs: map[string]interface{}{
				"prompt_variables": map[string]interface{}{"start.query": "show users"},
				"prompt":           map[string]interface{}{"user": "show users"},
				"system_prompt":    "hidden",
				"model":            map[string]interface{}{"name": "gpt"},
				"data_source":      map[string]interface{}{"id": "ds-1"},
				"schema_tables":    []interface{}{"main.users"},
				"table_schema":     []interface{}{map[string]interface{}{"id": "users", "name": "users"}},
			},
			want: map[string]interface{}{
				"prompt": map[string]interface{}{"user": "show users"},
			},
		},
		{
			name:     "Image generation: keep prompt and variables",
			nodeType: "image-gen",
			inputs: map[string]interface{}{
				"prompt_variables": map[string]interface{}{"start.subject": "cat"},
				"prompt":           "draw cat",
				"generation":       map[string]interface{}{"n": 1},
				"model":            map[string]interface{}{"name": "image"},
			},
			want: map[string]interface{}{
				"prompt_variables": map[string]interface{}{"start.subject": "cat"},
				"prompt":           "draw cat",
			},
		},
		{
			name:     "Loop: only keep loop configuration",
			nodeType: "loop",
			inputs: map[string]interface{}{
				"loop_variables":   map[string]interface{}{"i": 0},
				"loop_count":       3,
				"break_conditions": []interface{}{},
				"parallel_nums":    4,
				"sys.user_id":      "u1",
			},
			want: map[string]interface{}{
				"loop_variables":   map[string]interface{}{"i": 0},
				"loop_count":       3,
				"break_conditions": []interface{}{},
			},
		},
		{
			name:     "Iteration: keep selected variable values by variable name",
			nodeType: "iteration",
			inputs: map[string]interface{}{
				"iterator_selector": []interface{}{"start", "items"},
				"iterator_value":    []interface{}{"a", "b"},
				"output_selector":   []interface{}{"llm", "text"},
				"filelist":          []interface{}{"a", "b"},
				"text":              "done",
				"parallel_nums":     4,
			},
			want: map[string]interface{}{
				"filelist": []interface{}{"a", "b"},
				"text":     "done",
			},
		},
		{
			name:     "Variable assigner: only keep variable operation rules",
			nodeType: "assigner",
			inputs: map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"variable_selector": []interface{}{"conversation", "name"},
						"input_type":        "constant",
						"operation":         "over-write",
						"value":             "Alice",
					},
				},
				"updated_variables": map[string]interface{}{"name": "Alice"},
				"sys.user_id":       "u1",
			},
			want: map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"variable_selector": []interface{}{"conversation", "name"},
						"input_type":        "constant",
						"operation":         "over-write",
						"value":             "Alice",
					},
				},
			},
		},
		{
			name:     "Answer: keep selected variables and remove answer template",
			nodeType: "answer",
			inputs: map[string]interface{}{
				"start.query":  "hello",
				"sys.user_id":  "u1",
				"user_id":      "u2",
				"answer":       "{{#start.query#}}",
				"model_config": map[string]interface{}{"name": "hidden"},
			},
			want: map[string]interface{}{
				"query":       "hello",
				"sys.user_id": "u1",
				"user_id":     "u2",
				"model_config": map[string]interface{}{
					"name": "hidden",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterFrontendInputs(tt.nodeType, tt.inputs)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FilterFrontendInputs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterFrontendInputs_TargetWorkflowNodes(t *testing.T) {
	tests := []struct {
		name        string
		nodeType    string
		inputs      map[string]interface{}
		wantKeys    []string
		rejectKeys  []string
		wantKeyVals map[string]interface{}
	}{
		{
			name:     "HTTP request",
			nodeType: "http-request",
			inputs: map[string]interface{}{
				"url":    "http://baidu.com",
				"method": "GET",
				"header": map[string]interface{}{},
				"param":  map[string]interface{}{},
				"body":   nil,
				"auth":   nil,
				"timeout": map[string]interface{}{
					"read": 60,
				},
			},
			wantKeys:    []string{"url", "method", "header", "param", "auth"},
			rejectKeys:  []string{"body", "timeout"},
			wantKeyVals: map[string]interface{}{"url": "http://baidu.com", "method": "GET"},
		},
		{
			name:     "Call database",
			nodeType: "call-database",
			inputs: map[string]interface{}{
				"data_source":     map[string]interface{}{"id": "ds-1"},
				"schema_tables":   []interface{}{"ds-1.users"},
				"sql":             "SELECT 1",
				"table_selection": []interface{}{map[string]interface{}{"name": "users"}},
				"execution":       map[string]interface{}{"timeout_seconds": 30},
			},
			wantKeys:    []string{"sql"},
			rejectKeys:  []string{"data_source", "schema_tables", "execution", "table_selection"},
			wantKeyVals: map[string]interface{}{"sql": "SELECT 1"},
		},
		{
			name:     "SQL generator",
			nodeType: "sql-generator",
			inputs: map[string]interface{}{
				"prompt_variables": map[string]interface{}{"start.query": "show users"},
				"prompt":           map[string]interface{}{"user": "show users"},
				"system_prompt":    "hidden",
				"model":            map[string]interface{}{"name": "gpt"},
				"data_source":      map[string]interface{}{"id": "ds-1"},
				"schema_tables":    []interface{}{"ds-1.users"},
				"table_schema":     []interface{}{map[string]interface{}{"id": "users", "name": "users"}},
			},
			wantKeys:   []string{"prompt"},
			rejectKeys: []string{"prompt_variables", "system_prompt", "model", "data_source", "schema_tables", "table_schema"},
		},
		{
			name:     "Image generation",
			nodeType: "image-gen",
			inputs: map[string]interface{}{
				"prompt_variables": map[string]interface{}{"start.subject": "cat"},
				"prompt":           "draw cat",
				"model":            map[string]interface{}{"name": "image"},
				"generation":       map[string]interface{}{"n": 1},
			},
			wantKeys:   []string{"prompt_variables", "prompt"},
			rejectKeys: []string{"model", "generation"},
		},
		{
			name:     "Loop",
			nodeType: "loop",
			inputs: map[string]interface{}{
				"loop_variables":   map[string]interface{}{"i": 0},
				"loop_count":       3,
				"break_conditions": []interface{}{},
				"outputs":          map[string]interface{}{"answer": "hidden"},
			},
			wantKeys:    []string{"loop_variables", "loop_count", "break_conditions"},
			rejectKeys:  []string{"outputs"},
			wantKeyVals: map[string]interface{}{"loop_count": 3},
		},
		{
			name:     "Iteration",
			nodeType: "iteration",
			inputs: map[string]interface{}{
				"iterator_selector": []interface{}{"start", "items"},
				"iterator_value":    []interface{}{"a", "b"},
				"output_selector":   []interface{}{"llm", "text"},
				"filelist":          []interface{}{"a", "b"},
				"text":              "done",
				"parallel_nums":     4,
			},
			wantKeys:   []string{"filelist", "text"},
			rejectKeys: []string{"iterator_selector", "iterator_value", "output_selector", "parallel_nums"},
		},
		{
			name:     "Variable assigner",
			nodeType: "assigner",
			inputs: map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"variable_selector": []interface{}{"conversation", "name"},
						"input_type":        "constant",
						"operation":         "over-write",
						"value":             "Alice",
					},
				},
				"updated_variables": map[string]interface{}{"name": "Alice"},
				"process_data":      map[string]interface{}{"hidden": true},
			},
			wantKeys:   []string{"items"},
			rejectKeys: []string{"updated_variables", "process_data"},
		},
		{
			name:     "Answer",
			nodeType: "answer",
			inputs: map[string]interface{}{
				"llm.text":    "hello",
				"sys.user_id": "u1",
				"answer":      "{{#llm.text#}}",
				"streaming":   map[string]interface{}{"enabled": true},
			},
			wantKeys:    []string{"text", "sys.user_id"},
			rejectKeys:  []string{"llm.text", "answer", "streaming"},
			wantKeyVals: map[string]interface{}{"text": "hello"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterFrontendInputs(tt.nodeType, tt.inputs)
			for _, key := range tt.wantKeys {
				if _, exists := got[key]; !exists {
					t.Fatalf("expected key %s in filtered inputs: %#v", key, got)
				}
			}
			for _, key := range tt.rejectKeys {
				if _, exists := got[key]; exists {
					t.Fatalf("key %s should be removed from filtered inputs: %#v", key, got)
				}
			}
			for key, want := range tt.wantKeyVals {
				if !reflect.DeepEqual(got[key], want) {
					t.Fatalf("filtered input %s = %#v, want %#v", key, got[key], want)
				}
			}
		})
	}
}

func TestFilterFrontendInputs_ConfiguredSystemNamedVariablesAreKept(t *testing.T) {
	tests := []struct {
		name     string
		nodeType string
		inputs   map[string]interface{}
		want     map[string]interface{}
	}{
		{
			name:     "code configured variables keep system-like aliases",
			nodeType: "code",
			inputs: map[string]interface{}{
				"user_id":        "configured-user",
				"sys.user_id":    "configured-sys-user",
				"workspace_id":   "configured-workspace",
				"__trace_id__":   "internal",
				"model_config":   map[string]interface{}{"name": "hidden"},
				"business_value": "kept",
			},
			want: map[string]interface{}{
				"user_id":        "configured-user",
				"sys.user_id":    "configured-sys-user",
				"workspace_id":   "configured-workspace",
				"model_config":   map[string]interface{}{"name": "hidden"},
				"business_value": "kept",
			},
		},
		{
			name:     "parameter extractor keeps configured sys variable result",
			nodeType: "parameter-extractor",
			inputs: map[string]interface{}{
				"sys.user_id": "configured-sys-user",
				"user_id":     "configured-user",
				"query":       "extract",
			},
			want: map[string]interface{}{
				"sys.user_id": "configured-sys-user",
				"user_id":     "configured-user",
				"query":       "extract",
			},
		},
		{
			name:     "llm keeps prompt variables with system selectors",
			nodeType: "llm",
			inputs: map[string]interface{}{
				"prompt": "hello configured-user",
				"prompt_variables": map[string]interface{}{
					"sys.user_id": "configured-user",
					"user_id":     "configured-alias",
				},
			},
			want: map[string]interface{}{
				"prompt": "hello configured-user",
				"prompt_variables": map[string]interface{}{
					"sys.user_id": "configured-user",
					"user_id":     "configured-alias",
				},
			},
		},
		{
			name:     "loop keeps configured system-like loop variables",
			nodeType: "loop",
			inputs: map[string]interface{}{
				"loop_variables": map[string]interface{}{
					"sys.user_id":  "configured-user",
					"workspace_id": "configured-workspace",
				},
				"loop_count":       2,
				"break_conditions": []interface{}{},
				"sys.user_id":      "raw-system-context",
			},
			want: map[string]interface{}{
				"loop_variables": map[string]interface{}{
					"sys.user_id":  "configured-user",
					"workspace_id": "configured-workspace",
				},
				"loop_count":       2,
				"break_conditions": []interface{}{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterFrontendInputs(tt.nodeType, tt.inputs)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("FilterFrontendInputs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestFilterFrontendOutputs(t *testing.T) {
	tests := []struct {
		name     string
		nodeType string
		outputs  map[string]interface{}
		want     map[string]interface{}
	}{
		{
			name:     "remove sys keys from outputs for non-special nodes",
			nodeType: "llm",
			outputs: map[string]interface{}{
				"text":      "answer",
				"sys.query": "query",
			},
			want: map[string]interface{}{
				"text": "answer",
			},
		},
		{
			name:     "start node also filters sys keys from outputs",
			nodeType: "start",
			outputs: map[string]interface{}{
				"sys.query":  "hello",
				"user_input": "data",
			},
			want: map[string]interface{}{
				"user_input": "data",
			},
		},
		{
			name:     "remove internal keys",
			nodeType: "code",
			outputs: map[string]interface{}{
				"result":                 "ok",
				"__edge_source_handle__": "xxx",
			},
			want: map[string]interface{}{
				"result": "ok",
			},
		},
		{
			name:     "Parameter Extractor filters internal outputs",
			nodeType: "parameter-extractor",
			outputs: map[string]interface{}{
				"__is_success": 1,
				"__reason":     nil,
				"__usage":      "tokens",
				"param1":       "value1",
			},
			want: map[string]interface{}{
				"param1": "value1",
			},
		},
		{
			name:     "outputs use selector leaf variable name",
			nodeType: "answer",
			outputs: map[string]interface{}{
				"1779034444176_r5nt1.body": "request body",
				"llm.text":                 "answer",
			},
			want: map[string]interface{}{
				"body": "request body",
				"text": "answer",
			},
		},
		{
			name:     "remove internal keys and metadata",
			nodeType: "code",
			outputs: map[string]interface{}{
				"__internal__": "secret",
				"result":       "ok",
				"metadata":     "noise",
			},
			want: map[string]interface{}{
				"result": "ok",
			},
		},
		{
			name:     "LLM: filter finish_reason from outputs",
			nodeType: "llm",
			outputs: map[string]interface{}{
				"text":          "hello",
				"finish_reason": "stop",
			},
			want: map[string]interface{}{
				"text": "hello",
			},
		},
		{
			name:     "Image generation: keep only urls",
			nodeType: "image-gen",
			outputs: map[string]interface{}{
				"urls":            []interface{}{"https://example.com/a.png"},
				"files":           []interface{}{map[string]interface{}{"id": "file-1"}},
				"revised_prompts": []interface{}{"cat"},
			},
			want: map[string]interface{}{
				"urls": []interface{}{"https://example.com/a.png"},
			},
		},
		{
			name:     "Knowledge retrieval: trim retriever resources",
			nodeType: "knowledge-retrieval",
			outputs: map[string]interface{}{
				"retrieval.result": "context",
				"retriever_resources": []interface{}{
					map[string]interface{}{
						"document_id":      "doc-1",
						"document_name":    "Doc",
						"content":          "chunk",
						"score":            0.9,
						"dataset_id":       "dataset-1",
						"segment_position": 3,
					},
				},
			},
			want: map[string]interface{}{
				"result": "context",
				"retriever_resources": []interface{}{
					map[string]interface{}{
						"document_id":   "doc-1",
						"document_name": "Doc",
						"content":       "chunk",
						"score":         0.9,
					},
				},
			},
		},
		{
			name:     "Other nodes: keep finish_reason in outputs",
			nodeType: "code",
			outputs: map[string]interface{}{
				"result":        "docs",
				"finish_reason": "stop",
			},
			want: map[string]interface{}{
				"result":        "docs",
				"finish_reason": "stop",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterFrontendOutputs(tt.nodeType, tt.outputs)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FilterFrontendOutputs() = %v, want %v", got, tt.want)
			}
		})
	}
}
