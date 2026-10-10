package adapter

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNormalizeChatToolChoice(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  interface{}
	}{
		{name: "auto string", input: "auto", want: "auto"},
		{name: "required alias", input: "any", want: "required"},
		{name: "bare function name", input: "get_weather", want: map[string]interface{}{
			"type":     "function",
			"function": map[string]string{"name": "get_weather"},
		}},
		{name: "openai function object", input: map[string]interface{}{
			"type":     "function",
			"function": map[string]interface{}{"name": "get_weather"},
		}, want: map[string]interface{}{
			"type":     "function",
			"function": map[string]string{"name": "get_weather"},
		}},
		{name: "anthropic tool object", input: map[string]interface{}{
			"type": "tool",
			"name": "get_weather",
		}, want: map[string]interface{}{
			"type":     "function",
			"function": map[string]string{"name": "get_weather"},
		}},
		{name: "responses function object", input: map[string]interface{}{
			"type": "function",
			"name": "get_weather",
		}, want: map[string]interface{}{
			"type":     "function",
			"function": map[string]string{"name": "get_weather"},
		}},
		{name: "missing function name stays required", input: map[string]interface{}{
			"type": "function",
		}, want: "required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := NormalizeChatToolChoice(test.input)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("NormalizeChatToolChoice() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestCompatibleChatToolChoiceDowngradesNamedFunction(t *testing.T) {
	got := CompatibleChatToolChoice(map[string]interface{}{
		"type":     "function",
		"function": map[string]interface{}{"name": "get_weather"},
	})
	if got != "required" {
		t.Fatalf("CompatibleChatToolChoice() = %#v, want required", got)
	}
	if got := CompatibleChatToolChoice("auto"); got != "auto" {
		t.Fatalf("CompatibleChatToolChoice(auto) = %#v, want auto", got)
	}
}

func TestParseChatToolChoiceFromRawMessage(t *testing.T) {
	got := NormalizeChatToolChoice(json.RawMessage(`{"type":"function","function":{"name":"get_weather"}}`))
	want := map[string]interface{}{
		"type":     "function",
		"function": map[string]string{"name": "get_weather"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeChatToolChoice(raw) = %#v, want %#v", got, want)
	}
}
