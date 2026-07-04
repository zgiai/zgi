package gateway

import "testing"

func TestTokenEstimatorEstimateEmbeddingTokens(t *testing.T) {
	estimator := NewTokenEstimator()

	tests := []struct {
		name  string
		input interface{}
		want  int
	}{
		{name: "string", input: "abcd", want: 1},
		{name: "string slice", input: []string{"abcd", "abcdefgh"}, want: 3},
		{name: "token ids", input: []int{1, 2, 3}, want: 3},
		{name: "token id batches", input: [][]int{{1, 2}, {3, 4, 5}}, want: 5},
		{name: "interface token id batches", input: []interface{}{[]interface{}{float64(1), float64(2)}, []interface{}{float64(3)}}, want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimator.EstimateEmbeddingTokens(tt.input, "test-model")
			if got != tt.want {
				t.Fatalf("EstimateEmbeddingTokens() = %d, want %d", got, tt.want)
			}
		})
	}
}
