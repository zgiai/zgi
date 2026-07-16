package handler

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateDatasetNameCountsUnicodeCodePoints(t *testing.T) {
	handler := &DatasetHandler{}

	tests := []struct {
		name        string
		value       string
		wantErr     bool
		wantTooLong bool
	}{
		{name: "empty", value: "", wantErr: true},
		{name: "single character", value: "语"},
		{name: "reported multibyte name", value: "语文互联网测试AI分析智能体凡尔赛务工"},
		{name: "forty Chinese characters", value: strings.Repeat("语", datasetNameMaxLength)},
		{
			name:        "forty one Chinese characters",
			value:       strings.Repeat("语", datasetNameMaxLength+1),
			wantErr:     true,
			wantTooLong: true,
		},
		{name: "forty ASCII characters", value: strings.Repeat("a", datasetNameMaxLength)},
		{
			name:        "forty one ASCII characters",
			value:       strings.Repeat("a", datasetNameMaxLength+1),
			wantErr:     true,
			wantTooLong: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateName(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateName(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
			if got := errors.Is(err, errDatasetNameTooLong); got != tt.wantTooLong {
				t.Fatalf("validateName(%q) tooLong = %v, want %v", tt.value, got, tt.wantTooLong)
			}
		})
	}
}
