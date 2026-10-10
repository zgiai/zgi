package ratingcore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// The vectors are the frozen cross-repository contract. Both Console and Pre
// run this same test against a byte-identical mirrored copy; any behavioral
// drift fails CI here instead of in production billing.

const (
	vectorsPath  = "testdata/rating_vectors/rating-vectors-v1.json"
	registryPath = "testdata/rating_vectors/unit-registry-v1.json"
	sumsPath     = "testdata/rating_vectors/SHA256SUMS"
	testRateAt   = "2026-09-21T00:00:00Z"
)

type vectorsDocument struct {
	PointsPerUSD      int64          `json:"points_per_usd"`
	MilliPointsPerUSD int64          `json:"milli_points_per_usd"`
	Vectors           []ratingVector `json:"vectors"`
	EditorRoundtrip   []roundtripVec `json:"editor_roundtrip_vectors"`
}

type ratingVector struct {
	ID              string  `json:"id"`
	Description     string  `json:"description"`
	Lane            string  `json:"lane"`
	Rate            *string `json:"rate"`
	RateSource      string  `json:"rate_source"`
	DisplayCurrency string  `json:"display_currency"`
	Pricing         Pricing `json:"pricing"`
	Usage           Usage   `json:"usage"`
	Expected        struct {
		USD          *json.RawMessage `json:"usd"`
		UsdTolerance string           `json:"usd_tolerance"`
		PointsMP     *int64           `json:"points_mp"`
		TotalPoints  *json.RawMessage `json:"total_points"`
		Error        *struct {
			Code      string `json:"code"`
			HTTP      int    `json:"http"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	} `json:"expected"`
}

type roundtripVec struct {
	ID                string `json:"id"`
	Currency          string `json:"currency"`
	Rate              string `json:"rate"`
	SubmittedAmount   string `json:"submitted_amount"`
	ExpectStoredUSD   string `json:"expect_stored_usd"`
	ExpectPopulated   string `json:"expect_populated_amount"`
	ExpectResubmitUSD string `json:"expect_resubmit_stored_usd"`
}

func loadVectors(t *testing.T) vectorsDocument {
	t.Helper()
	raw, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var document vectorsDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode vectors: %v", err)
	}
	if len(document.Vectors) == 0 {
		t.Fatal("vectors document contains no vectors")
	}
	return document
}

func TestGoldenRatingVectors(t *testing.T) {
	document := loadVectors(t)
	if document.PointsPerUSD != PointsPerUSD || document.MilliPointsPerUSD != MilliPointsPerUSD {
		t.Fatalf("vector constants (%d/%d) do not match ratingcore (%d/%d)",
			document.PointsPerUSD, document.MilliPointsPerUSD, PointsPerUSD, MilliPointsPerUSD)
	}

	for _, vector := range document.Vectors {
		vector := vector
		t.Run(vector.ID, func(t *testing.T) {
			request := Request{
				Pricing:         vector.Pricing,
				Usage:           vector.Usage,
				Rate:            parseRate(vector.Rate),
				RateSource:      vector.RateSource,
				RateAt:          testRateAt,
				DisplayCurrency: vector.DisplayCurrency,
			}
			result, err := Quote(request)

			if vector.Expected.Error != nil {
				if err == nil {
					t.Fatalf("expected error %s, got result %s", vector.Expected.Error.Code, result.USD.String())
				}
				var ratingError *Error
				if !errors.As(err, &ratingError) {
					t.Fatalf("expected *ratingcore.Error, got %T: %v", err, err)
				}
				if ratingError.Code != vector.Expected.Error.Code {
					t.Fatalf("error code = %q, want %q", ratingError.Code, vector.Expected.Error.Code)
				}
				if ratingError.HTTPStatus != vector.Expected.Error.HTTP {
					t.Fatalf("error HTTP = %d, want %d", ratingError.HTTPStatus, vector.Expected.Error.HTTP)
				}
				if ratingError.Retryable != vector.Expected.Error.Retryable {
					t.Fatalf("error retryable = %v, want %v", ratingError.Retryable, vector.Expected.Error.Retryable)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if vector.Expected.PointsMP == nil {
				t.Fatal("successful vector is missing expected.points_mp")
			}
			if result.PointsMP != *vector.Expected.PointsMP {
				t.Fatalf("points_mp = %d, want %d", result.PointsMP, *vector.Expected.PointsMP)
			}
			usdTolerance := vector.Expected.UsdTolerance
			if usdTolerance == "" {
				// comparison_rules defaults the USD comparison to 1e-15;
				// points_mp remains the exact accounting assertion.
				usdTolerance = "1e-15"
			}
			assertDecimalString(t, "usd", result.USD, vector.Expected.USD, usdTolerance)
			assertDecimalString(t, "total_points", result.TotalPoints, vector.Expected.TotalPoints, "")
			assertSnapshot(t, result.Snapshot, vector)
		})
	}
}

func assertSnapshot(t *testing.T, snapshot Snapshot, vector ratingVector) {
	t.Helper()
	if snapshot.Rate == "" || snapshot.RateSource == "" || snapshot.RateAt == "" || snapshot.DisplayCurrency == "" {
		t.Fatalf("snapshot has empty required fields: %+v", snapshot)
	}
	if vector.Rate != nil && snapshot.Rate != *vector.Rate {
		t.Fatalf("snapshot rate = %q, want %q", snapshot.Rate, *vector.Rate)
	}
	if snapshot.RateSource != vector.RateSource {
		t.Fatalf("snapshot rate_source = %q, want %q", snapshot.RateSource, vector.RateSource)
	}
	if snapshot.DisplayCurrency != DisplayCurrency(vector.DisplayCurrency) {
		t.Fatalf("snapshot display_currency = %q, want %q", snapshot.DisplayCurrency, vector.DisplayCurrency)
	}
}

func parseRate(value *string) *decimal.Decimal {
	if value == nil {
		return nil
	}
	rate, err := decimal.NewFromString(*value)
	if err != nil {
		return nil
	}
	return &rate
}

func assertDecimalString(t *testing.T, field string, actual decimal.Decimal, raw *json.RawMessage, tolerance string) {
	t.Helper()
	if raw == nil {
		t.Fatalf("%s: expected value is missing", field)
	}
	expected, err := decodeDecimal(*raw)
	if err != nil {
		t.Fatalf("%s: decode expected: %v", field, err)
	}
	if tolerance == "" {
		if !actual.Equal(expected) {
			t.Fatalf("%s = %s, want %s", field, actual.String(), expected.String())
		}
		return
	}
	allowed, err := decimal.NewFromString(tolerance)
	if err != nil {
		t.Fatalf("%s: invalid tolerance %q: %v", field, tolerance, err)
	}
	if actual.Sub(expected).Abs().GreaterThan(allowed) {
		t.Fatalf("%s = %s, want %s (tolerance %s)", field, actual.String(), expected.String(), tolerance)
	}
}

func decodeDecimal(raw json.RawMessage) (decimal.Decimal, error) {
	if string(raw) == "null" {
		return decimal.Zero, errors.New("null decimal")
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return decimal.NewFromString(asString)
	}
	var asNumber json.Number
	if err := json.Unmarshal(raw, &asNumber); err == nil {
		return decimal.NewFromString(asNumber.String())
	}
	return decimal.Zero, errors.New("unsupported decimal encoding")
}

// TestEditorRoundtripVectors locks the "convert once" invariant: a submitted
// amount converts to canonical USD once and populates back without a second
// division.
func TestEditorRoundtripVectors(t *testing.T) {
	document := loadVectors(t)
	if len(document.EditorRoundtrip) == 0 {
		t.Fatal("vectors document contains no editor roundtrip vectors")
	}
	for _, vector := range document.EditorRoundtrip {
		vector := vector
		t.Run(vector.ID, func(t *testing.T) {
			rate, err := decimal.NewFromString(vector.Rate)
			if err != nil {
				t.Fatalf("rate: %v", err)
			}
			submitted, err := decimal.NewFromString(vector.SubmittedAmount)
			if err != nil {
				t.Fatalf("submitted: %v", err)
			}
			stored, err := ConvertToUSD(submitted, vector.Currency, &rate)
			if err != nil {
				t.Fatalf("convert to USD: %v", err)
			}
			assertDecimalEqualString(t, "stored USD", stored, vector.ExpectStoredUSD)

			populated, err := ConvertFromUSD(stored, vector.Currency, &rate)
			if err != nil {
				t.Fatalf("convert from USD: %v", err)
			}
			assertDecimalEqualString(t, "populated amount", populated, vector.ExpectPopulated)

			resubmitted, err := ConvertToUSD(populated, vector.Currency, &rate)
			if err != nil {
				t.Fatalf("resubmit to USD: %v", err)
			}
			assertDecimalEqualString(t, "resubmitted USD", resubmitted, vector.ExpectResubmitUSD)
		})
	}
}

func assertDecimalEqualString(t *testing.T, field string, actual decimal.Decimal, expected string) {
	t.Helper()
	want, err := decimal.NewFromString(expected)
	if err != nil {
		t.Fatalf("%s: parse expected %q: %v", field, expected, err)
	}
	if !actual.Equal(want) {
		t.Fatalf("%s = %s, want %s", field, actual.String(), want.String())
	}
}

func TestUnitRegistryMirrorsContract(t *testing.T) {
	raw, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("read unit registry: %v", err)
	}
	var document struct {
		PointsPerUSD        int64 `json:"points_per_usd"`
		MilliPointsPerPoint int64 `json:"milli_points_per_point"`
		MilliPointsPerUSD   int64 `json:"milli_points_per_usd"`
		Entries             []struct {
			Operation          string `json:"operation"`
			Meter              string `json:"meter"`
			Unit               string `json:"unit"`
			BaseUnit           string `json:"base_unit"`
			PerQuantityDefault int64  `json:"per_quantity_default"`
			QuantityKind       string `json:"quantity_kind"`
			PricingSource      string `json:"pricing_source"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode unit registry: %v", err)
	}
	if document.PointsPerUSD != PointsPerUSD || document.MilliPointsPerPoint != MilliPointsPerPoint || document.MilliPointsPerUSD != MilliPointsPerUSD {
		t.Fatalf("registry constants do not match ratingcore")
	}

	actual := UnitDefinitions()
	if len(actual) != len(document.Entries) {
		t.Fatalf("registry entry count = %d, want %d", len(actual), len(document.Entries))
	}
	byKey := make(map[string]UnitDefinition, len(actual))
	for _, entry := range actual {
		byKey[entry.Operation+"|"+entry.Meter] = entry
	}
	for _, expected := range document.Entries {
		got, ok := byKey[expected.Operation+"|"+expected.Meter]
		if !ok {
			t.Fatalf("registry is missing %s/%s", expected.Operation, expected.Meter)
		}
		if got.Unit != expected.Unit || got.BaseUnit != expected.BaseUnit ||
			got.PerQuantityDefault != expected.PerQuantityDefault ||
			got.QuantityKind != expected.QuantityKind || got.PricingSource != expected.PricingSource {
			t.Fatalf("registry row %s/%s = %+v, want %+v", expected.Operation, expected.Meter, got, expected)
		}
	}
}

func TestMirroredContractHashes(t *testing.T) {
	sumsRaw, err := os.ReadFile(sumsPath)
	if err != nil {
		t.Fatalf("read SHA256SUMS: %v", err)
	}
	seen := 0
	for _, line := range strings.Split(strings.TrimSpace(string(sumsRaw)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("malformed SHA256SUMS line %q", line)
		}
		expected, name := fields[0], fields[1]
		content, err := os.ReadFile(filepath.Join(filepath.Dir(sumsPath), name))
		if err != nil {
			t.Fatalf("read mirror %s: %v", name, err)
		}
		sum := sha256.Sum256(content)
		if hex.EncodeToString(sum[:]) != expected {
			t.Fatalf("mirror %s hash drifted from the frozen lock file", name)
		}
		seen++
	}
	if seen == 0 {
		t.Fatal("SHA256SUMS contains no entries")
	}
}

func TestRegistryTablesAreUnique(t *testing.T) {
	seen := make(map[string]struct{})
	for _, entry := range unitRegistry {
		key := entry.Operation + "|" + entry.Meter
		if _, exists := seen[key]; exists {
			t.Fatalf("duplicate registry key %s", key)
		}
		seen[key] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) != len(unitRegistry) {
		t.Fatalf("registry key count = %d, entries = %d", len(keys), len(unitRegistry))
	}
}

func TestConvertToUSDFailsClosed(t *testing.T) {
	amount := decimal.NewFromInt(7)
	if _, err := ConvertToUSD(amount, "CNY", nil); err == nil {
		t.Fatal("expected missing CNY rate to fail closed")
	}
	if _, err := ConvertToUSD(amount, "EUR", nil); err == nil {
		t.Fatal("expected unsupported currency to fail closed")
	}
	usd, err := ConvertToUSD(amount, "USD", nil)
	if err != nil || !usd.Equal(amount) {
		t.Fatalf("USD passthrough = %s, %v", usd, err)
	}
}
