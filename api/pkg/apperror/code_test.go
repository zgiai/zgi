package apperror_test

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/pkg/apperror"
)

func TestParseCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "canonical", value: "llm.provider.timeout", valid: true},
		{name: "underscores and digits", value: "workflow.node_v2.execution_failed", valid: true},
		{name: "two segments", value: "internal.unclassified", valid: true},
		{name: "empty", value: "", valid: false},
		{name: "one segment", value: "timeout", valid: false},
		{name: "uppercase", value: "LLM.provider.timeout", valid: false},
		{name: "leading digit", value: "1llm.provider.timeout", valid: false},
		{name: "leading underscore", value: "_llm.provider.timeout", valid: false},
		{name: "trailing underscore", value: "llm.provider_.timeout", valid: false},
		{name: "double underscore", value: "llm.provider__api.timeout", valid: false},
		{name: "empty segment", value: "llm..timeout", valid: false},
		{name: "hyphen", value: "llm.provider-rate.timeout", valid: false},
		{name: "space", value: " llm.provider.timeout", valid: false},
		{name: "unicode", value: "模型.供应商.超时", valid: false},
		{name: "too long", value: "domain." + strings.Repeat("a", 122), valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			code, err := apperror.ParseCode(test.value)
			if test.valid {
				if err != nil {
					t.Fatalf("ParseCode(%q) returned error: %v", test.value, err)
				}
				if code.String() != test.value || !code.Valid() {
					t.Fatalf("ParseCode(%q) = %q, valid=%v", test.value, code, code.Valid())
				}
				return
			}
			if err == nil {
				t.Fatalf("ParseCode(%q) unexpectedly succeeded with %q", test.value, code)
			}
			if code.String() != "" {
				t.Fatalf("invalid ParseCode(%q) returned code %q", test.value, code)
			}
		})
	}
}

func TestMustCode(t *testing.T) {
	t.Parallel()

	if got := apperror.MustCode("billing.workspace.quota_exceeded"); got.String() != "billing.workspace.quota_exceeded" {
		t.Fatalf("MustCode returned %q", got)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("MustCode did not panic for an invalid code")
		}
	}()
	_ = apperror.MustCode("invalid")
}

func FuzzParseCode(f *testing.F) {
	for _, seed := range []string{
		"llm.provider.timeout",
		"internal.unclassified",
		"",
		"LLM.provider.timeout",
		"llm..timeout",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, value string) {
		code, err := apperror.ParseCode(value)
		if err != nil {
			if code.String() != "" {
				t.Fatalf("invalid input returned non-empty code %q", code)
			}
			return
		}
		if code.String() != value {
			t.Fatalf("ParseCode changed %q to %q", value, code)
		}
		if validateErr := code.Validate(); validateErr != nil {
			t.Fatalf("ParseCode returned invalid code %q: %v", code, validateErr)
		}
	})
}
