package code

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

func TestNode_ExecuteRun_UsesNestedSelector(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"source", "payload"}, map[string]any{
		"value": "deep",
	})

	var gotInputValue string
	prevTransport := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		_ = r.Body.Close()

		var payload struct {
			Language string `json:"language"`
			Code     string `json:"code"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}
		if payload.Language != "python3" {
			t.Fatalf("language = %q, want python3", payload.Language)
		}

		matches := regexp.MustCompile(`b64decode\('([^']+)'\)`).FindStringSubmatch(payload.Code)
		if len(matches) != 2 {
			t.Fatalf("encoded inputs not found in runner code")
		}

		inputBytes, err := base64.StdEncoding.DecodeString(matches[1])
		if err != nil {
			t.Fatalf("decode inputs: %v", err)
		}

		var inputs map[string]any
		if err := json.Unmarshal(inputBytes, &inputs); err != nil {
			t.Fatalf("unmarshal inputs: %v", err)
		}

		gotInputValue, _ = inputs["input_value"].(string)

		respBody, err := json.Marshal(map[string]any{
			"code":    0,
			"message": "ok",
			"data": map[string]any{
				"stdout": "<<RESULT>>{\"output\":{\"result\":\"deep\"}}<<RESULT>>",
			},
		})
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(string(respBody))),
		}, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = prevTransport
	})

	prevConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		CodeExec: config.CodeExecConfig{
			Endpoint: "http://code-exec.test",
			APIKey:   "test-key",
		},
	}
	t.Cleanup(func() {
		config.GlobalConfig = prevConfig
	})

	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID:            "code-node-1",
			GraphRuntimeState: entities.NewGraphRuntimeState(vp),
		},
		NodeData: NodeData{
			Variables: []VariableSelector{
				{
					Variable:      "input_value",
					ValueSelector: []string{"source", "payload", "value"},
				},
			},
			CodeLanguage: CodeLanguagePython3,
			Code: `def main(input_value=None):
    return {"output": {"result": input_value}}
`,
			Outputs: map[string]Output{
				"output": {
					Type: shared.SegmentTypeObject,
					Children: map[string]*Output{
						"result": {Type: shared.SegmentTypeString},
					},
				},
			},
		},
	}

	result, err := node.executeRun(context.Background(), nil)
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}

	if gotInputValue != "deep" {
		t.Fatalf("sandbox input_value = %q, want deep", gotInputValue)
	}

	output, ok := result.Outputs["output"].(map[string]any)
	if !ok {
		t.Fatalf("output type = %T, want map[string]any", result.Outputs["output"])
	}
	if output["result"] != "deep" {
		t.Fatalf("output result = %v, want deep", output["result"])
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
