package handler

import "testing"

func TestNormalizeUploadProcessingMode(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "defaults to process now", raw: "", want: UploadProcessingModeProcessNow, ok: true},
		{name: "trims whitespace", raw: "  store_only  ", want: UploadProcessingModeStoreOnly, ok: true},
		{name: "accepts process now", raw: UploadProcessingModeProcessNow, want: UploadProcessingModeProcessNow, ok: true},
		{name: "rejects invalid", raw: "parse_later", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizeUploadProcessingMode(tt.raw)
			if ok != tt.ok {
				t.Fatalf("ok=%v want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("mode=%q want %q", got, tt.want)
			}
		})
	}
}
