package service

import (
	"errors"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestNormalizeModelParametersAcceptsReasoningEffort(t *testing.T) {
	params, err := normalizeModelParameters(map[string]interface{}{"reasoning_effort": " High "})
	if err != nil {
		t.Fatalf("normalizeModelParameters() error = %v", err)
	}
	if params["reasoning_effort"] != "high" {
		t.Fatalf("reasoning_effort = %#v, want %q", params["reasoning_effort"], "high")
	}
	for _, invalid := range []interface{}{"max", "", 1, true} {
		_, err := normalizeModelParameters(map[string]interface{}{"reasoning_effort": invalid})
		if !errors.Is(err, ErrInvalidModelParam) {
			t.Fatalf("reasoning_effort=%#v: error = %v, want ErrInvalidModelParam", invalid, err)
		}
	}
}

func TestApplyModelParametersForwardsReasoningEffortAsExtra(t *testing.T) {
	req := &adapter.ChatRequest{}
	applyModelParameters(req, map[string]interface{}{"reasoning_effort": "none", "temperature": 0.5})
	if req.AdditionalParameters["reasoning_effort"] != "none" {
		t.Fatalf("AdditionalParameters = %#v, want reasoning_effort=none", req.AdditionalParameters)
	}
	if req.Temperature == nil || *req.Temperature != 0.5 {
		t.Fatalf("temperature = %v, want 0.5", req.Temperature)
	}

	untouched := &adapter.ChatRequest{}
	applyModelParameters(untouched, map[string]interface{}{"temperature": 0.5})
	if untouched.AdditionalParameters != nil {
		t.Fatalf("AdditionalParameters = %#v, want nil when reasoning_effort is not set", untouched.AdditionalParameters)
	}
}
