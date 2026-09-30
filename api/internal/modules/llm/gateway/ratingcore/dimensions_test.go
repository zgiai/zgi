package ratingcore

import (
	"testing"

	"github.com/shopspring/decimal"
)

func decPtr(value string) *decimal.Decimal {
	parsed := decimal.RequireFromString(value)
	return &parsed
}

func boolPtr(value bool) *bool { return &value }

func videoSecondPricing(rates ...VideoRate) Pricing {
	return Pricing{VideoGeneration: &VideoGenerationPricing{Rates: rates}}
}

func rateFor(usage Usage, pricing Pricing) (decimal.Decimal, error) {
	return Resolve(pricing, usage)
}

// Report M2: same resolution, different audio price. Before the fix both
// requests were rated on the first row (0.1/s) and the audio one was under-billed.
func TestVideoRateSelectionSeparatesAudioFromSilent(t *testing.T) {
	pricing := videoSecondPricing(
		VideoRate{ID: "silent", Resolution: "720p", Audio: boolPtr(false), PricePerSecond: decPtr("0.1")},
		VideoRate{ID: "audio", Resolution: "720p", Audio: boolPtr(true), PricePerSecond: decPtr("0.4")},
	)
	base := Usage{Operation: "video_generation", Meter: "output_video_second", BaseUnit: "second", Quantity: 10}

	silent := base
	silent.Dimensions = Dimensions{Resolution: "720p", Audio: boolPtr(false)}
	if got, err := rateFor(silent, pricing); err != nil || !got.Equal(decimal.RequireFromString("1")) {
		t.Fatalf("silent = %v err=%v, want 1", got, err)
	}
	withAudio := base
	withAudio.Dimensions = Dimensions{Resolution: "720p", Audio: boolPtr(true)}
	if got, err := rateFor(withAudio, pricing); err != nil || !got.Equal(decimal.RequireFromString("4")) {
		t.Fatalf("audio = %v err=%v, want 4", got, err)
	}
	// An omitted flag reads as false, like an omitted request field.
	omitted := base
	omitted.Dimensions = Dimensions{Resolution: "720p"}
	if got, err := rateFor(omitted, pricing); err != nil || !got.Equal(decimal.RequireFromString("1")) {
		t.Fatalf("omitted audio = %v err=%v, want 1", got, err)
	}
}

func TestVideoRateSelectionSeparatesInputVideo(t *testing.T) {
	pricing := videoSecondPricing(
		VideoRate{ID: "no-input", InputVideo: boolPtr(false), PricePerSecond: decPtr("2")},
		VideoRate{ID: "with-input", InputVideo: boolPtr(true), PricePerSecond: decPtr("1")},
	)
	usage := Usage{Operation: "video_generation", Meter: "output_video_second", BaseUnit: "second", Quantity: 5,
		Dimensions: Dimensions{InputVideo: boolPtr(true)}}
	if got, err := rateFor(usage, pricing); err != nil || !got.Equal(decimal.RequireFromString("5")) {
		t.Fatalf("input video = %v err=%v, want 5", got, err)
	}
}

func TestVideoRateSelectionRejectsDurationMismatch(t *testing.T) {
	pricing := videoSecondPricing(VideoRate{ID: "d8", DurationSeconds: decPtr("8"), PricePerSecond: decPtr("1")})
	usage := Usage{Operation: "video_generation", Meter: "output_video_second", BaseUnit: "second", Quantity: 10,
		Dimensions: Dimensions{DurationSeconds: decPtr("10")}}
	if _, err := rateFor(usage, pricing); err == nil {
		t.Fatal("a duration the rate does not declare must fail closed")
	}
	usage.Dimensions.DurationSeconds = decPtr("8")
	if _, err := rateFor(usage, pricing); err != nil {
		t.Fatalf("matching duration must be rated: %v", err)
	}
}

func TestVideoRateSelectionKeepsResolutionAndTierTie(t *testing.T) {
	// Historical behavior: resolution and model_tier carry equal weight.
	pricing := videoSecondPricing(
		VideoRate{ID: "by-res", Resolution: "720p", PricePerSecond: decPtr("1")},
		VideoRate{ID: "by-tier", ModelTier: "pro", PricePerSecond: decPtr("9")},
	)
	rate, ok := selectVideoRate(pricing.VideoGeneration.Rates, Dimensions{Resolution: "720p", ModelTier: "pro"})
	if !ok || rate.ID != "by-res" {
		t.Fatalf("tie must keep the first row, got %q ok=%v", rate.ID, ok)
	}
}

// Report M6: a realtime session bills text tokens on chat rules and audio
// tokens on realtime_audio rules; the audio price no longer replaces the text.
func TestRealtimeAudioBillsTextAndAudioTokensTogether(t *testing.T) {
	pricing := Pricing{Rules: []Rule{
		{ID: "rt-in", Operation: "realtime_audio", Meter: "input_token", Unit: "per_1m_tokens", Price: decimal.RequireFromString("32")},
		{ID: "rt-out", Operation: "realtime_audio", Meter: "output_token", Unit: "per_1m_tokens", Price: decimal.RequireFromString("64")},
		{ID: "chat-in", Operation: "chat", Meter: "input_token", Unit: "per_1m_tokens", Price: decimal.RequireFromString("4")},
		{ID: "chat-out", Operation: "chat", Meter: "output_token", Unit: "per_1m_tokens", Price: decimal.RequireFromString("16")},
	}}
	usage := Usage{Operation: "realtime_audio",
		AudioInputTokens: 1_000_000, AudioOutputTokens: 1_000_000,
		TextInputTokens: 1_000_000, TextOutputTokens: 1_000_000}
	got, err := Resolve(pricing, usage)
	if err != nil {
		t.Fatal(err)
	}
	// 32 + 64 audio, 4 + 16 text.
	if !got.Equal(decimal.RequireFromString("116")) {
		t.Fatalf("realtime total = %s, want 116", got)
	}
}

func TestRealtimeAudioTextTokensFailClosedWithoutChatRule(t *testing.T) {
	pricing := Pricing{Rules: []Rule{
		{ID: "rt-in", Operation: "realtime_audio", Meter: "input_token", Unit: "per_1m_tokens", Price: decimal.RequireFromString("32")},
	}}
	usage := Usage{Operation: "realtime_audio", AudioInputTokens: 1000, TextInputTokens: 1000}
	if _, err := Resolve(pricing, usage); err == nil {
		t.Fatal("text tokens without a chat rule must fail closed, not vanish")
	}
}

// A realtime session's text cache reads are priced on chat/cache_read_token and
// its audio cache reads on realtime_audio/cache_read_token; they add up.
func TestRealtimeAudioSplitsTextAndAudioCacheReads(t *testing.T) {
	pricing := Pricing{Rules: []Rule{
		{ID: "rt-cr", Operation: "realtime_audio", Meter: "cache_read_token", Unit: "per_1m_tokens", Price: decimal.RequireFromString("0.4")},
		{ID: "chat-cr", Operation: "chat", Meter: "cache_read_token", Unit: "per_1m_tokens", Price: decimal.RequireFromString("0.1")},
	}}
	usage := Usage{Operation: "realtime_audio", CacheReadTokens: 1_000_000, TextCacheReadTokens: 2_000_000}
	got, err := Resolve(pricing, usage)
	if err != nil {
		t.Fatal(err)
	}
	// 1M audio @ 0.4 + 2M text @ 0.1.
	if !got.Equal(decimal.RequireFromString("0.6")) {
		t.Fatalf("realtime cache total = %s, want 0.6", got)
	}
	if _, err := Resolve(Pricing{Rules: pricing.Rules[:1]}, usage); err == nil {
		t.Fatal("text cache reads without a chat rule must fail closed")
	}
}
