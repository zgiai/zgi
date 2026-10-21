package ratingcore

// UnitDefinition is one frozen row of unit-registry-v1.json. operation x meter maps
// to exactly one unit/base_unit; unknown combinations must be rejected rather
// than guessed.
type UnitDefinition struct {
	Operation          string
	Meter              string
	Unit               string
	BaseUnit           string
	PerQuantityDefault int64
	QuantityKind       string
	PricingSource      string
}

// unitRegistry is the single source of truth for legal pricing combinations.
// It mirrors unit-registry-v1.json exactly; RatingCore registry tests assert the
// mirror so a drift fails CI instead of silently rating a wrong unit.
var unitRegistry = []UnitDefinition{
	{Operation: "chat", Meter: "input_token", Unit: "per_1m_tokens", BaseUnit: "per_1m_tokens", PerQuantityDefault: 1000000, QuantityKind: "tokens", PricingSource: "rules"},
	{Operation: "chat", Meter: "output_token", Unit: "per_1m_tokens", BaseUnit: "per_1m_tokens", PerQuantityDefault: 1000000, QuantityKind: "tokens", PricingSource: "rules"},
	{Operation: "chat", Meter: "cache_read_token", Unit: "per_1m_tokens", BaseUnit: "per_1m_tokens", PerQuantityDefault: 1000000, QuantityKind: "tokens", PricingSource: "rules"},
	{Operation: "chat", Meter: "cache_write_token", Unit: "per_1m_tokens", BaseUnit: "per_1m_tokens", PerQuantityDefault: 1000000, QuantityKind: "tokens", PricingSource: "rules"},
	{Operation: "chat", Meter: "cache_write_5m_token", Unit: "per_1m_tokens", BaseUnit: "per_1m_tokens", PerQuantityDefault: 1000000, QuantityKind: "tokens", PricingSource: "rules"},
	{Operation: "chat", Meter: "cache_write_1h_token", Unit: "per_1m_tokens", BaseUnit: "per_1m_tokens", PerQuantityDefault: 1000000, QuantityKind: "tokens", PricingSource: "rules"},
	{Operation: "embedding", Meter: "input_token", Unit: "per_1m_tokens", BaseUnit: "per_1m_tokens", PerQuantityDefault: 1000000, QuantityKind: "tokens", PricingSource: "rules"},
	{Operation: "rerank", Meter: "input_token", Unit: "per_1m_tokens", BaseUnit: "per_1m_tokens", PerQuantityDefault: 1000000, QuantityKind: "tokens", PricingSource: "rules"},
	{Operation: "realtime_audio", Meter: "input_token", Unit: "per_1m_tokens", BaseUnit: "per_1m_tokens", PerQuantityDefault: 1000000, QuantityKind: "audio_tokens", PricingSource: "rules"},
	{Operation: "realtime_audio", Meter: "output_token", Unit: "per_1m_tokens", BaseUnit: "per_1m_tokens", PerQuantityDefault: 1000000, QuantityKind: "audio_tokens", PricingSource: "rules"},
	{Operation: "realtime_audio", Meter: "cache_read_token", Unit: "per_1m_tokens", BaseUnit: "per_1m_tokens", PerQuantityDefault: 1000000, QuantityKind: "audio_tokens", PricingSource: "rules"},
	{Operation: "image_generation", Meter: "image", Unit: "per_image", BaseUnit: "per_image", PerQuantityDefault: 1, QuantityKind: "images", PricingSource: "rules"},
	{Operation: "image_generation", Meter: "input_text_token", Unit: "per_1m_tokens", BaseUnit: "per_1m_tokens", PerQuantityDefault: 1000000, QuantityKind: "tokens", PricingSource: "image_token_pricing"},
	{Operation: "image_generation", Meter: "input_image_token", Unit: "per_1m_tokens", BaseUnit: "per_1m_tokens", PerQuantityDefault: 1000000, QuantityKind: "tokens", PricingSource: "image_token_pricing"},
	{Operation: "image_generation", Meter: "output_token", Unit: "per_1m_tokens", BaseUnit: "per_1m_tokens", PerQuantityDefault: 1000000, QuantityKind: "tokens", PricingSource: "image_token_pricing"},
	{Operation: "video_generation", Meter: "output_video_second", Unit: "second", BaseUnit: "second", PerQuantityDefault: 1, QuantityKind: "seconds", PricingSource: "video_generation"},
	{Operation: "video_generation", Meter: "output_video_task", Unit: "task", BaseUnit: "task", PerQuantityDefault: 1, QuantityKind: "tasks", PricingSource: "video_generation"},
	{Operation: "video_generation", Meter: "output_video_token", Unit: "million_video_tokens", BaseUnit: "million_video_tokens", PerQuantityDefault: 1000000, QuantityKind: "video_tokens", PricingSource: "video_generation"},
	{Operation: "transcription", Meter: "input_audio_duration", Unit: "millisecond", BaseUnit: "millisecond", PerQuantityDefault: 1, QuantityKind: "milliseconds", PricingSource: "metered"},
	{Operation: "speech_generation", Meter: "input_text", Unit: "billed_character", BaseUnit: "billed_character", PerQuantityDefault: 1, QuantityKind: "characters", PricingSource: "metered"},
	{Operation: "music_generation", Meter: "output_track", Unit: "track", BaseUnit: "track", PerQuantityDefault: 1, QuantityKind: "tracks", PricingSource: "metered"},
	{Operation: "lyrics_generation", Meter: "request", Unit: "request", BaseUnit: "request", PerQuantityDefault: 1, QuantityKind: "requests", PricingSource: "metered"},
	{Operation: "rerank", Meter: "search", Unit: "search", BaseUnit: "search", PerQuantityDefault: 1, QuantityKind: "searches", PricingSource: "metered"},
	{Operation: "embedding", Meter: "input_pixel", Unit: "per_1m_pixels", BaseUnit: "per_1m_pixels", PerQuantityDefault: 1000000, QuantityKind: "pixels", PricingSource: "metered"},
	{Operation: "transcription", Meter: "input_token", Unit: "per_1m_tokens", BaseUnit: "per_1m_tokens", PerQuantityDefault: 1000000, QuantityKind: "tokens", PricingSource: "metered"},
	{Operation: "transcription", Meter: "output_token", Unit: "per_1m_tokens", BaseUnit: "per_1m_tokens", PerQuantityDefault: 1000000, QuantityKind: "tokens", PricingSource: "metered"},
	{Operation: "speech_generation", Meter: "output_audio_token", Unit: "per_1m_tokens", BaseUnit: "per_1m_tokens", PerQuantityDefault: 1000000, QuantityKind: "audio_tokens", PricingSource: "metered"},
	{Operation: "music_generation", Meter: "output_duration", Unit: "minute", BaseUnit: "minute", PerQuantityDefault: 1, QuantityKind: "minutes", PricingSource: "metered"},
	{Operation: "web_search", Meter: "call", Unit: "call", BaseUnit: "call", PerQuantityDefault: 1, QuantityKind: "calls", PricingSource: "metered"},
	{Operation: "file_search", Meter: "call", Unit: "call", BaseUnit: "call", PerQuantityDefault: 1, QuantityKind: "calls", PricingSource: "metered"},
	{Operation: "code_interpreter", Meter: "session", Unit: "session", BaseUnit: "session", PerQuantityDefault: 1, QuantityKind: "sessions", PricingSource: "metered"},
	{Operation: "code_interpreter", Meter: "hour", Unit: "hour", BaseUnit: "hour", PerQuantityDefault: 1, QuantityKind: "hours", PricingSource: "metered"},
	{Operation: "chat", Meter: "input_image_token", Unit: "per_1m_tokens", BaseUnit: "per_1m_tokens", PerQuantityDefault: 1000000, QuantityKind: "tokens", PricingSource: "metered"},
	{Operation: "image_generation", Meter: "input_text_token_cached", Unit: "per_1m_tokens", BaseUnit: "per_1m_tokens", PerQuantityDefault: 1000000, QuantityKind: "tokens", PricingSource: "metered"},
	{Operation: "image_generation", Meter: "input_image_token_cached", Unit: "per_1m_tokens", BaseUnit: "per_1m_tokens", PerQuantityDefault: 1000000, QuantityKind: "tokens", PricingSource: "metered"},
}

// UnitDefinitions returns a defensive copy of the frozen registry.
func UnitDefinitions() []UnitDefinition {
	out := make([]UnitDefinition, len(unitRegistry))
	copy(out, unitRegistry)
	return out
}

// LookupUnit returns the registry row for operation x meter. It reports false
// for out-of-table combinations so callers fail closed instead of guessing.
func LookupUnit(operation, meter string) (UnitDefinition, bool) {
	for _, entry := range unitRegistry {
		if entry.Operation == operation && entry.Meter == meter {
			return entry, true
		}
	}
	return UnitDefinition{}, false
}

// unitForBaseUnit returns the registry row for operation x base_unit.
func unitForBaseUnit(operation, baseUnit string) (UnitDefinition, bool) {
	for _, entry := range unitRegistry {
		if entry.Operation == operation && entry.BaseUnit == baseUnit {
			return entry, true
		}
	}
	return UnitDefinition{}, false
}

// knownMeter reports whether operation x meter x baseUnit is a frozen registry
// combination.
func knownMeter(operation, meter, baseUnit string) bool {
	entry, ok := LookupUnit(operation, meter)
	return ok && entry.BaseUnit == baseUnit
}
