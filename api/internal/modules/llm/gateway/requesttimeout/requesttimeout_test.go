package requesttimeout

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"
)

func resetLimits(t *testing.T, max, def string) {
	t.Helper()
	t.Setenv(maxEnvKey, max)
	t.Setenv(defaultEnvKey, def)
	limitsOnce = sync.Once{}
	t.Cleanup(func() { limitsOnce = sync.Once{} })
}

func silenceWarnings(t *testing.T) *[]string {
	t.Helper()
	var warnings []string
	previous := warn
	warn = func(msg string) { warnings = append(warnings, msg) }
	t.Cleanup(func() { warn = previous })
	return &warnings
}

func TestParseRequestTimeout(t *testing.T) {
	resetLimits(t, "", "")
	cases := []struct {
		raw  string
		want time.Duration
		ok   bool
	}{
		{"1200", 1200 * time.Second, true},
		{" 60 ", 60 * time.Second, true},
		{"1", time.Second, true},
		{"3600", 3600 * time.Second, true},
		{"", 0, false},
		{"0", 0, false},
		{"-1", 0, false},
		{"+60", 0, false},
		{"1.5", 0, false},
		{"60s", 0, false},
		{"abc", 0, false},
		{"3601", 0, false},
		{"99999999999999999999", 0, false},
	}
	for _, tc := range cases {
		got, err := Parse(tc.raw)
		if tc.ok {
			if err != nil || got != tc.want {
				t.Fatalf("Parse(%q) = %s, %v; want %s", tc.raw, got, err, tc.want)
			}
			continue
		}
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("Parse(%q) error = %v, want ErrInvalid", tc.raw, err)
		}
	}
}

func TestLimitsFromEnv(t *testing.T) {
	cases := []struct {
		name, max, def  string
		wantMax, wantDf time.Duration
		wantWarnings    int
	}{
		{"defaults", "", "", 3600 * time.Second, 1200 * time.Second, 0},
		{"custom", "7200", "1800", 7200 * time.Second, 1800 * time.Second, 0},
		{"invalid max", "zero", "", 3600 * time.Second, 1200 * time.Second, 1},
		{"default below min", "", "10", 3600 * time.Second, 1200 * time.Second, 1},
		{"default above max", "900", "", 900 * time.Second, 900 * time.Second, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			warnings := silenceWarnings(t)
			resetLimits(t, tc.max, tc.def)
			if Max() != tc.wantMax || Default() != tc.wantDf {
				t.Fatalf("Max() = %s, Default() = %s; want %s, %s", Max(), Default(), tc.wantMax, tc.wantDf)
			}
			if len(*warnings) != tc.wantWarnings {
				t.Fatalf("warnings = %v, want %d", *warnings, tc.wantWarnings)
			}
		})
	}
}

func TestRequestedAndEffective(t *testing.T) {
	resetLimits(t, "", "")
	if _, ok := Requested(context.Background()); ok {
		t.Fatal("empty context reported a requested timeout")
	}
	if got := Effective(context.Background()); got != Default() {
		t.Fatalf("Effective(empty) = %s, want default %s", got, Default())
	}
	ctx := WithTimeout(context.Background(), 2400*time.Second)
	if got, ok := Requested(ctx); !ok || got != 2400*time.Second {
		t.Fatalf("Requested() = %s, %v", got, ok)
	}
	if got := Effective(ctx); got != 2400*time.Second {
		t.Fatalf("Effective() = %s, want 2400s", got)
	}
}

func TestAppendOutgoingMetadataSendsEffectiveSeconds(t *testing.T) {
	resetLimits(t, "", "")
	cases := []struct {
		ctx  context.Context
		want string
	}{
		{WithTimeout(context.Background(), 1800*time.Second), "1800"},
		{context.Background(), "1200"},
	}
	for _, tc := range cases {
		md, ok := metadata.FromOutgoingContext(AppendOutgoingMetadata(tc.ctx))
		if !ok || len(md.Get(MetadataKey)) != 1 || md.Get(MetadataKey)[0] != tc.want {
			t.Fatalf("metadata = %v, want %s=%s", md, MetadataKey, tc.want)
		}
	}
	// Existing outgoing metadata is kept.
	base := metadata.AppendToOutgoingContext(context.Background(), "x-existing", "1")
	md, _ := metadata.FromOutgoingContext(AppendOutgoingMetadata(base))
	if md.Get("x-existing")[0] != "1" {
		t.Fatalf("existing metadata lost: %v", md)
	}
}
