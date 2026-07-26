package skillloop

import (
	"errors"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestNormalizeSkillToolArgumentsAcceptsObject(t *testing.T) {
	input := map[string]interface{}{
		"arguments": map[string]interface{}{
			"content":  "# Report",
			"format":   "md",
			"filename": "report",
		},
	}

	got, err := normalizeSkillToolArguments(input, skills.SkillFileGenerator, "generate_file")
	if err != nil {
		t.Fatalf("normalizeSkillToolArguments() error = %v", err)
	}
	if got["content"] != "# Report" || got["format"] != "md" || got["filename"] != "report" {
		t.Fatalf("normalized arguments = %#v", got)
	}
}

func TestNormalizeSkillToolArgumentsAcceptsOneLayerJSONString(t *testing.T) {
	input := map[string]interface{}{
		"arguments": `{"content":"# Report","format":"md","filename":"report"}`,
	}

	got, err := normalizeSkillToolArguments(input, skills.SkillFileGenerator, "generate_file")
	if err != nil {
		t.Fatalf("normalizeSkillToolArguments() error = %v", err)
	}
	if got["content"] != "# Report" || got["format"] != "md" || got["filename"] != "report" {
		t.Fatalf("normalized arguments = %#v", got)
	}
}

func TestNormalizeSkillToolArgumentsRejectsInvalidJSONWithoutRepair(t *testing.T) {
	input := map[string]interface{}{
		"arguments": `{"content":"# Report","format":"md"`,
	}

	_, err := normalizeSkillToolArguments(input, skills.SkillFileGenerator, "generate_file")
	assertSkillToolArgumentsErrorCode(t, err, skillToolArgumentsInvalidJSONCode)
}

func TestNormalizeSkillToolArgumentsRejectsNonObjectJSON(t *testing.T) {
	input := map[string]interface{}{
		"arguments": `["# Report","md"]`,
	}

	_, err := normalizeSkillToolArguments(input, skills.SkillFileGenerator, "generate_file")
	argumentsErr := assertSkillToolArgumentsErrorCode(t, err, skillToolArgumentsWrongTypeCode)
	if argumentsErr.ActualType != "array" {
		t.Fatalf("actual type = %q, want array", argumentsErr.ActualType)
	}
}

func TestNormalizeSkillToolArgumentsReportsMissingFields(t *testing.T) {
	input := map[string]interface{}{
		"arguments": map[string]interface{}{"format": "md"},
	}

	_, err := normalizeSkillToolArguments(input, skills.SkillFileGenerator, "generate_file")
	argumentsErr := assertSkillToolArgumentsErrorCode(t, err, skillToolArgumentsMissingCode)
	if len(argumentsErr.MissingFields) != 1 || argumentsErr.MissingFields[0] != "content" {
		t.Fatalf("missing fields = %#v, want content", argumentsErr.MissingFields)
	}
	if argumentsErr.ExpectedArguments["schema"] == nil {
		t.Fatalf("expected arguments = %#v, want schema", argumentsErr.ExpectedArguments)
	}
}

func TestNormalizeSkillToolArgumentsRejectsOversizedString(t *testing.T) {
	input := map[string]interface{}{
		"arguments": strings.Repeat("x", skillToolArgumentsMaxJSONBytes+1),
	}

	_, err := normalizeSkillToolArguments(input, skills.SkillFileGenerator, "generate_file")
	assertSkillToolArgumentsErrorCode(t, err, skillToolArgumentsInvalidJSONCode)
}

func TestNormalizeSkillToolArgumentsRejectsExcessiveDepth(t *testing.T) {
	value := interface{}("leaf")
	for i := 0; i < skillToolArgumentsMaxJSONDepth; i++ {
		value = map[string]interface{}{"nested": value}
	}
	input := map[string]interface{}{
		"arguments": map[string]interface{}{
			"content": value,
			"format":  "md",
		},
	}

	_, err := normalizeSkillToolArguments(input, skills.SkillFileGenerator, "generate_file")
	assertSkillToolArgumentsErrorCode(t, err, skillToolArgumentsInvalidJSONCode)
}

func assertSkillToolArgumentsErrorCode(t *testing.T, err error, code string) *skillToolArgumentsError {
	t.Helper()
	if err == nil {
		t.Fatalf("normalizeSkillToolArguments() error = nil, want %s", code)
	}
	var argumentsErr *skillToolArgumentsError
	if !errors.As(err, &argumentsErr) {
		t.Fatalf("error type = %T, want *skillToolArgumentsError", err)
	}
	if argumentsErr.Code != code {
		t.Fatalf("error code = %q, want %q", argumentsErr.Code, code)
	}
	return argumentsErr
}
