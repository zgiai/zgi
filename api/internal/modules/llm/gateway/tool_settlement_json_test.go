package gateway

import "testing"

func TestT4MirrorJSONComparesExactNumbersAcrossPostgresEncoding(t *testing.T) {
	for _, tt := range []struct {
		name, a, b string
		equal      bool
	}{
		{"actual supplier snapshot", `{"token_quote":{"sale_amount_usd":1.600008573e-7}}`, `{"token_quote":{"sale_amount_usd":0.0000001600008573}}`, true},
		{"large integer", `{"x":9007199254740993}`, `{"x":9007199254740993.0}`, true},
		{"different large integer", `{"x":9007199254740993}`, `{"x":9007199254740992}`, false},
		{"different precise fraction", `{"x":0.0000001600008573}`, `{"x":0.0000001600008574}`, false},
		{"numeric string remains string", `{"x":"1"}`, `{"x":1}`, false},
		{"nested arrays", `{"unknown":[1e2,-0.0,{"x":1.00}]}`, `{"unknown":[100,0,{"x":1}]}`, true},
		{"unknown fields conflict", `{"x":1,"future":true}`, `{"x":1}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := toolJSONEqual([]byte(tt.a), []byte(tt.b)); got != tt.equal {
				t.Fatalf("equal=%v want %v", got, tt.equal)
			}
		})
	}
}
