package config

import "testing"

func TestLoadUploadConfigEnforcesMinimumFileSizeLimit(t *testing.T) {
	tests := []struct {
		name  string
		value string
		set   bool
		want  int
	}{
		{name: "default", want: 50},
		{name: "below minimum", value: "15", set: true, want: 50},
		{name: "minimum", value: "50", set: true, want: 50},
		{name: "above minimum", value: "75", set: true, want: 75},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := &envSource{lookupEnv: func(key string) (string, bool) {
				if tt.set && key == envUploadFileSizeLimit {
					return tt.value, true
				}
				return "", false
			}}
			cfg := &Config{}

			if err := loadUploadConfig(cfg, source); err != nil {
				t.Fatalf("loadUploadConfig() error = %v", err)
			}
			if cfg.Upload.FileSizeLimit != tt.want {
				t.Fatalf("FileSizeLimit = %d, want %d", cfg.Upload.FileSizeLimit, tt.want)
			}
		})
	}
}
