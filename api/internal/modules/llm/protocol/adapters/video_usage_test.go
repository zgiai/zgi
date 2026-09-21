package adapter

import "testing"

// T13: the upstream actual video duration must be read from the provider
// response only. Missing duration must stay zero so settlement can fail closed.
func TestVideoResponseActualDurationReadsTypedAndRawKeys(t *testing.T) {
	cases := []struct {
		name string
		resp *VideoResponse
		want float64
	}{
		{name: "typed field", resp: &VideoResponse{ActualDurationSeconds: 8}, want: 8},
		{name: "raw duration_seconds", resp: &VideoResponse{Raw: map[string]interface{}{"duration_seconds": float64(9)}}, want: 9},
		{name: "raw duration int", resp: &VideoResponse{Raw: map[string]interface{}{"duration": 6}}, want: 6},
		{name: "raw camel case", resp: &VideoResponse{Raw: map[string]interface{}{"durationSeconds": 7}}, want: 7},
		{name: "nested output", resp: &VideoResponse{Raw: map[string]interface{}{"output": map[string]interface{}{"video_duration": 11}}}, want: 11},
		{name: "string value", resp: &VideoResponse{Raw: map[string]interface{}{"duration": "4.5"}}, want: 4.5},
		{name: "absent", resp: &VideoResponse{Raw: map[string]interface{}{"status": "succeeded"}}, want: 0},
		{name: "zero is not reported", resp: &VideoResponse{Raw: map[string]interface{}{"duration": 0}}, want: 0},
		{name: "nil response", resp: nil, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.resp.VideoActualDurationSeconds(); got != tc.want {
				t.Fatalf("VideoActualDurationSeconds() = %v, want %v", got, tc.want)
			}
		})
	}
}
