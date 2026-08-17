package metatools

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

func TestNormalizeExecuteActionParametersAcceptsNativeAndEncodedObjects(t *testing.T) {
	tests := []struct {
		name      string
		arguments interface{}
	}{
		{name: "native", arguments: map[string]interface{}{"keyword": "Yang"}},
		{name: "encoded", arguments: `{"keyword":"Yang","page":2}`},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			normalized, err := normalizeExecuteActionParameters(map[string]interface{}{
				"integration_id": "dingtalk",
				"action_id":      "dingtalk.contact.search",
				"arguments":      testCase.arguments,
			})
			if err != nil {
				t.Fatalf("normalizeExecuteActionParameters() error = %v", err)
			}
			arguments, ok := normalized["arguments"].(map[string]interface{})
			if !ok || arguments["keyword"] != "Yang" {
				t.Fatalf("normalized arguments = %#v", normalized["arguments"])
			}
			if testCase.name == "encoded" && arguments["page"] != float64(2) {
				t.Fatalf("normalized numeric argument = %#v", arguments["page"])
			}
		})
	}
}

func TestNormalizeExecuteActionParametersRejectsUnsafeRepresentations(t *testing.T) {
	tests := []struct {
		name      string
		arguments interface{}
	}{
		{name: "empty", arguments: "  "},
		{name: "invalid_json", arguments: `{"keyword":`},
		{name: "trailing_json", arguments: `{"keyword":"Yang"} {}`},
		{name: "encoded_array", arguments: `["Yang"]`},
		{name: "encoded_scalar", arguments: `"Yang"`},
		{name: "native_array", arguments: []interface{}{"Yang"}},
		{name: "null", arguments: nil},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := normalizeExecuteActionParameters(map[string]interface{}{"arguments": testCase.arguments})
			if err == nil || integrations.ErrorCode(err) != integrations.ErrorCodeInvalidInput {
				t.Fatalf("error = %v, code = %q", err, integrations.ErrorCode(err))
			}
			recoveryProvider, ok := err.(interface{ PublicErrorRecovery() map[string]interface{} })
			if !ok {
				t.Fatalf("error does not expose structural recovery: %T", err)
			}
			recovery := recoveryProvider.PublicErrorRecovery()
			if recovery["reason_code"] != executeActionArgumentsEncodingReason ||
				recovery["provider_request_sent"] != false || recovery["expected_type"] != "object" {
				t.Fatalf("recovery = %#v", recovery)
			}
			if strings.Contains(fmt.Sprint(recovery), "Yang") {
				t.Fatalf("recovery leaked rejected argument value: %#v", recovery)
			}
		})
	}
}

func TestNormalizeExecuteActionParametersEnforcesBounds(t *testing.T) {
	t.Run("bytes", func(t *testing.T) {
		_, err := normalizeExecuteActionParameters(map[string]interface{}{
			"arguments": map[string]interface{}{"value": strings.Repeat("x", maxExecuteActionArgumentsJSONBytes)},
		})
		if err == nil {
			t.Fatal("oversized arguments were accepted")
		}
	})

	t.Run("depth", func(t *testing.T) {
		root := map[string]interface{}{}
		cursor := root
		for depth := 0; depth < maxExecuteActionArgumentsJSONDepth+1; depth++ {
			nested := map[string]interface{}{}
			cursor["nested"] = nested
			cursor = nested
		}
		if _, err := normalizeExecuteActionParameters(map[string]interface{}{"arguments": root}); err == nil {
			t.Fatal("over-depth arguments were accepted")
		}
	})

	t.Run("fields", func(t *testing.T) {
		arguments := make(map[string]interface{}, maxExecuteActionArgumentsJSONFields+1)
		for index := 0; index <= maxExecuteActionArgumentsJSONFields; index++ {
			arguments[fmt.Sprintf("field_%04d", index)] = index
		}
		if _, err := normalizeExecuteActionParameters(map[string]interface{}{"arguments": arguments}); err == nil {
			t.Fatal("over-field-count arguments were accepted")
		}
	})
}

func TestNormalizeExecuteActionParametersLeavesOmittedArgumentsOmitted(t *testing.T) {
	input := map[string]interface{}{"integration_id": "github", "action_id": "github.user.get"}
	normalized, err := normalizeExecuteActionParameters(input)
	if err != nil {
		t.Fatalf("normalizeExecuteActionParameters() error = %v", err)
	}
	if _, exists := normalized["arguments"]; exists {
		t.Fatalf("omitted arguments were synthesized: %#v", normalized)
	}
}
