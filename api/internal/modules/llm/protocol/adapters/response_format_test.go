package adapter

import (
	"encoding/json"
	"testing"
)

const openAIJSONSchemaChatBody = `{
	"model":"gpt-test",
	"messages":[{"role":"user","content":"hi"}],
	"response_format":{"type":"json_schema","json_schema":{"name":"weather","strict":true,
		"schema":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"],"additionalProperties":false}}}
}`

func TestChatRequestKeepsOpenAIJSONSchemaResponseFormat(t *testing.T) {
	var request ChatRequest
	if err := json.Unmarshal([]byte(openAIJSONSchemaChatBody), &request); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	schema := request.ResponseFormat.SchemaDefinition()
	if schema["type"] != "object" {
		t.Fatalf("SchemaDefinition() = %#v, want the json_schema.schema object", schema)
	}

	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded struct {
		ResponseFormat map[string]any `json:"response_format"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(payload) error = %v", err)
	}
	spec, ok := decoded.ResponseFormat["json_schema"].(map[string]any)
	if !ok {
		t.Fatalf("response_format = %#v, want json_schema object forwarded", decoded.ResponseFormat)
	}
	if spec["name"] != "weather" || spec["strict"] != true || spec["schema"] == nil {
		t.Fatalf("json_schema = %#v, want name, strict and schema preserved", spec)
	}
	if _, ok := decoded.ResponseFormat["schema"]; ok {
		t.Fatalf("response_format = %#v, want no top-level schema for json_schema", decoded.ResponseFormat)
	}
}

func TestResponseFormatMarshalsInternalSchemaAsOpenAIJSONSchema(t *testing.T) {
	format := ResponseFormat{Type: "json_schema", Schema: map[string]any{"type": "object"}}
	payload, err := json.Marshal(format)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	want := `{"type":"json_schema","json_schema":{"name":"response","schema":{"type":"object"}}}`
	if string(payload) != want {
		t.Fatalf("json.Marshal() = %s, want %s", payload, want)
	}
}

func TestResponseFormatMarshalKeepsLegacyShapeForOtherTypes(t *testing.T) {
	payload, err := json.Marshal(&ResponseFormat{Type: "json_object"})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if string(payload) != `{"type":"json_object"}` {
		t.Fatalf("json.Marshal() = %s, want plain json_object", payload)
	}
}
