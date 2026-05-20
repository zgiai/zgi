package export

import "testing"

// 回归测试：BBOX_ALIGNMENT_PLAN.md §1.2 里 "多个 chunk 共用同一 bbox" 的场景
// 必须被 ClassifyBBoxReliability 识别并降级，而不是被原样下发给前端。
func TestClassifyBBoxReliability_DowngradesShared(t *testing.T) {
	shared := map[string]any{
		"left": 0.768, "top": 0.015, "right": 1.0, "bottom": 0.055,
	}
	boxByChunk := map[string]map[string]any{}
	// 24 个 chunk 共用同一个 bbox（对应方案文档里观察到的数据模式）
	for i := 0; i < 24; i++ {
		boxByChunk[chunkID(i)] = shared
	}
	// 1 个 chunk 有独立合法 bbox
	boxByChunk["good"] = map[string]any{"left": 0.1, "top": 0.2, "right": 0.8, "bottom": 0.25}

	rep, unreliable := ClassifyBBoxReliability(boxByChunk)

	if rep.TotalChunks != 25 {
		t.Fatalf("TotalChunks=%d want 25", rep.TotalChunks)
	}
	if rep.WithBox != 25 {
		t.Fatalf("WithBox=%d want 25", rep.WithBox)
	}
	if rep.SharedBoxOver3 != 1 {
		t.Errorf("SharedBoxOver3=%d want 1 (one key shared by >3)", rep.SharedBoxOver3)
	}
	if rep.GeomReliable != 1 {
		t.Errorf("GeomReliable=%d want 1 (only the 'good' chunk)", rep.GeomReliable)
	}
	if !unreliable[chunkID(0)] {
		t.Errorf("expected shared chunk %q to be marked unreliable", chunkID(0))
	}
	if unreliable["good"] {
		t.Errorf("did not expect good chunk to be marked unreliable")
	}
}

func TestClassifyBBoxReliability_GeomInvalid(t *testing.T) {
	cases := map[string]map[string]any{
		"zero_width":   {"left": 1.0, "top": 0.5, "right": 1.0, "bottom": 0.6},
		"huge_area":    {"left": 0.0, "top": 0.0, "right": 1.0, "bottom": 1.0}, // area > 0.72
		"out_of_bound": {"left": -0.05, "top": 0.1, "right": 0.3, "bottom": 0.2},
	}
	rep, unreliable := ClassifyBBoxReliability(cases)
	if rep.GeomReliable != 0 {
		t.Fatalf("GeomReliable=%d want 0", rep.GeomReliable)
	}
	for id := range cases {
		if !unreliable[id] {
			t.Errorf("%s should be unreliable", id)
		}
	}
}

func TestIsBBoxReliableGeom(t *testing.T) {
	tests := []struct {
		name string
		box  Box
		want bool
	}{
		{"normal", Box{0.1, 0.2, 0.5, 0.3}, true},
		{"zero_width", Box{0.5, 0.2, 0.5, 0.3}, false},
		{"zero_height", Box{0.1, 0.3, 0.5, 0.3}, false},
		{"inverted", Box{0.5, 0.3, 0.1, 0.2}, false},
		{"huge", Box{0.0, 0.0, 1.0, 0.9}, false},
		{"tiny", Box{0.1, 0.2, 0.1001, 0.2001}, false},
		{"slightly_over_bound", Box{0.0, 0.0, 1.02, 0.05}, false},
	}
	for _, tc := range tests {
		if got := IsBBoxReliableGeom(tc.box); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func chunkID(i int) string {
	return "c-" + itoaSmall(i)
}

func itoaSmall(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [8]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
