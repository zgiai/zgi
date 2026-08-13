package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeConfigParametersJSONPreservesOptionLabels(t *testing.T) {
	raw := []byte(`[{"name":"default_voice","template_key":"default_voice","type":"string","required":true,"options":["voice-1"],"option_labels":{"voice-1":{"zh_Hans":"音色一","en_US":"Voice One"}}}]`)

	params, err := NormalizeConfigParametersJSON(raw)
	if err != nil {
		t.Fatalf("NormalizeConfigParametersJSON() error = %v", err)
	}

	label := params[0].OptionLabels["voice-1"]
	if got, want := label.ZhHans, "音色一"; got != want {
		t.Fatalf("zh_Hans = %q, want %q", got, want)
	}
	if got, want := label.EnUS, "Voice One"; got != want {
		t.Fatalf("en_US = %q, want %q", got, want)
	}

	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(encoded), `"option_labels":{"voice-1"`) {
		t.Fatalf("encoded parameters dropped option_labels: %s", encoded)
	}
}

func TestValidateConfigParametersRejectsLabelForUnknownOption(t *testing.T) {
	err := ValidateConfigParameters(ConfigParameters{
		{
			Name:        "default_voice",
			TemplateKey: "default_voice",
			Type:        "string",
			Options:     []string{"voice-1"},
			OptionLabels: map[string]ConfigParameterLocalizedText{
				"voice-2": {ZhHans: "音色二"},
			},
		},
	})

	if err == nil || !strings.Contains(err.Error(), "unknown option") {
		t.Fatalf("ValidateConfigParameters() error = %v, want unknown option", err)
	}
}
