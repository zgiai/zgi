package envconfig

import (
	"testing"
	"time"
)

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

func TestWithOverridesSerializesRequestScopedValues(t *testing.T) {
	const key = "CONTENT_PARSE_ENVCONFIG_TEST_CONCURRENT_OVERRIDE"
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstResult := make(chan string, 1)
	secondStarted := make(chan struct{})
	secondEntered := make(chan struct{})
	secondDone := make(chan struct{})

	go func() {
		_ = WithExclusiveOverridesResult(map[string]string{key: "first"}, func() error {
			close(firstEntered)
			<-releaseFirst
			firstResult <- String(key)
			return nil
		})
	}()
	<-firstEntered

	go func() {
		close(secondStarted)
		_ = WithExclusiveOverridesResult(map[string]string{key: "second"}, func() error {
			close(secondEntered)
			return nil
		})
		close(secondDone)
	}()
	<-secondStarted

	select {
	case <-secondEntered:
		t.Fatal("second override entered before the first request completed")
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseFirst)

	if got := <-firstResult; got != "first" {
		t.Fatalf("first request observed %q, want first", got)
	}
	select {
	case <-secondEntered:
	case <-time.After(time.Second):
		t.Fatal("second override did not proceed after the first request completed")
	}
	<-secondDone
}
