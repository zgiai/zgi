package middleware

import (
	"reflect"
	"testing"
)

func TestSanitizedAPIKeyUsageHeaderRedactsSensitiveHeaders(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		values []string
		want   interface{}
	}{
		{
			name:   "authorization",
			key:    "Authorization",
			values: []string{"Bearer zgi_secret"},
			want:   "Bearer ***",
		},
		{
			name:   "authorization lowercase",
			key:    "authorization",
			values: []string{"Bearer zgi_secret"},
			want:   "Bearer ***",
		},
		{
			name:   "x api key",
			key:    "X-API-Key",
			values: []string{"zgi_secret"},
			want:   "***",
		},
		{
			name:   "x api key lowercase",
			key:    "x-api-key",
			values: []string{"zgi_secret"},
			want:   "***",
		},
		{
			name:   "ordinary header",
			key:    "User-Agent",
			values: []string{"test-client"},
			want:   []string{"test-client"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizedAPIKeyUsageHeader(tt.key, tt.values)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("sanitizedAPIKeyUsageHeader() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
