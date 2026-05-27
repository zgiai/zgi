package envconfig

import "testing"

func TestStringUsesTestEnvironmentOverride(t *testing.T) {
	t.Setenv("CONTENT_PARSE_ENVCONFIG_TEST_KEY", " from-test ")

	if got := String("CONTENT_PARSE_ENVCONFIG_TEST_KEY"); got != "from-test" {
		t.Fatalf("String() = %q, want from-test", got)
	}
}

func TestIntAndBoolUseConfigString(t *testing.T) {
	t.Setenv("CONTENT_PARSE_ENVCONFIG_TEST_INT", "42")
	t.Setenv("CONTENT_PARSE_ENVCONFIG_TEST_BOOL", "true")

	if got := Int("CONTENT_PARSE_ENVCONFIG_TEST_INT", 7); got != 42 {
		t.Fatalf("Int() = %d, want 42", got)
	}
	if got := Bool("CONTENT_PARSE_ENVCONFIG_TEST_BOOL", false); !got {
		t.Fatalf("Bool() = false, want true")
	}
}

func TestWithOverridesTakesPrecedenceAndRestores(t *testing.T) {
	t.Setenv("CONTENT_PARSE_ENVCONFIG_TEST_OVERRIDE", "from-env")

	WithOverrides(map[string]string{
		"CONTENT_PARSE_ENVCONFIG_TEST_OVERRIDE": "from-override",
	}, func() {
		if got := String("CONTENT_PARSE_ENVCONFIG_TEST_OVERRIDE"); got != "from-override" {
			t.Fatalf("String() inside override = %q, want from-override", got)
		}
	})

	if got := String("CONTENT_PARSE_ENVCONFIG_TEST_OVERRIDE"); got != "from-env" {
		t.Fatalf("String() after override = %q, want from-env", got)
	}
}
