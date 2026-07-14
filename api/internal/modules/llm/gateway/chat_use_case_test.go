package gateway

import "testing"

func TestModelUseCaseForAppContext(t *testing.T) {
	tests := []struct {
		appType string
		want    string
	}{
		{appType: "agent", want: "agent"},
		{appType: "aichat", want: "agent"},
		{appType: "workflow", want: "text-chat"},
		{appType: "dataset", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.appType, func(t *testing.T) {
			appType := tt.appType
			if got := modelUseCaseForAppContext(&AppContext{AppType: &appType}); got != tt.want {
				t.Fatalf("modelUseCaseForAppContext(%q) = %q, want %q", tt.appType, got, tt.want)
			}
		})
	}
}
