package indexing

import "testing"

func TestAllTargetsTerminal(t *testing.T) {
	tests := []struct {
		name     string
		ready    int
		terminal int
		total    int
		want     bool
	}{
		{name: "pending target remains", ready: 1, terminal: 1, total: 3, want: false},
		{name: "all targets ready", ready: 3, terminal: 0, total: 3, want: true},
		{name: "ready and failed targets complete", ready: 2, terminal: 1, total: 3, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := allTargetsTerminal(tt.ready, tt.terminal, tt.total); got != tt.want {
				t.Fatalf("allTargetsTerminal(%d, %d, %d) = %v, want %v", tt.ready, tt.terminal, tt.total, got, tt.want)
			}
		})
	}
}
