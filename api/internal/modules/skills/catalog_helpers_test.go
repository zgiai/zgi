package skills

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSummarizeArgumentsRedactsStructuredArtifactContent(t *testing.T) {
	const secret = "private report body"
	arguments := map[string]interface{}{
		"document": map[string]interface{}{
			"blocks": []interface{}{map[string]interface{}{"type": "paragraph", "text": secret}},
		},
		"presentation": `{"slides":[{"elements":[{"type":"text","text":"private slide body"}]}]}`,
		"filename":     "report",
	}

	summary := summarizeArguments(arguments)
	encoded := strings.ToLower(strings.TrimSpace(toJSONString(summary)))
	if strings.Contains(encoded, secret) || strings.Contains(encoded, "private slide body") {
		t.Fatalf("summarized arguments leaked structured content: %s", encoded)
	}
	document, ok := summary["document"].(map[string]interface{})
	if !ok {
		t.Fatalf("document summary type = %T, want map", summary["document"])
	}
	if document["argument_type"] != "object" || document["block_count"] != 1 {
		t.Fatalf("document summary = %#v", document)
	}
	presentation, ok := summary["presentation"].(map[string]interface{})
	if !ok {
		t.Fatalf("presentation summary type = %T, want map", summary["presentation"])
	}
	if presentation["argument_type"] != "string" || presentation["slide_count"] != 1 {
		t.Fatalf("presentation summary = %#v", presentation)
	}
}

func toJSONString(value interface{}) string {
	data, _ := json.Marshal(value)
	return string(data)
}
