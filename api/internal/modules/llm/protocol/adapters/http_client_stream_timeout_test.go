package adapter

import (
	"testing"
	"time"
)

// A streaming call waits for response headers at least 600s, and as long as
// the request allows when that is longer (the Console hop of a 2400s request).
func TestStreamResponseHeaderTimeoutFollowsLongRequests(t *testing.T) {
	cases := []struct{ timeout, want time.Duration }{
		{0, 600 * time.Second},
		{120 * time.Second, 600 * time.Second},
		{500 * time.Second, 600 * time.Second},
		{2430 * time.Second, 2430 * time.Second},
	}
	for _, tc := range cases {
		if got := streamResponseHeaderTimeout(tc.timeout); got != tc.want {
			t.Fatalf("streamResponseHeaderTimeout(%s) = %s, want %s", tc.timeout, got, tc.want)
		}
	}
}
