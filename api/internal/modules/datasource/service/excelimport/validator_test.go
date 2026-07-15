package excelimport

import "testing"

func TestConvertValueAcceptsExcelShortDate(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{raw: "04-15-23", want: "2023-04-15 00:00:00"},
		{raw: "08-01-22", want: "2022-08-01 00:00:00"},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := convertValue(tt.raw, "timestamp", true)
			if err != nil {
				t.Fatalf("convertValue returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("convertValue = %v, want %s", got, tt.want)
			}
		})
	}
}
