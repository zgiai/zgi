package config

import "testing"

func TestLoadFeatureConfigPhoneLogin(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "disabled by default", want: false},
		{name: "enabled", value: "true", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := &envSource{lookupEnv: func(key string) (string, bool) {
				if key == envEnablePhoneLogin && tt.value != "" {
					return tt.value, true
				}
				return "", false
			}}
			cfg := &Config{}

			loadFeatureConfig(cfg, source)

			if got := cfg.Feature.EnablePhoneLogin; got != tt.want {
				t.Fatalf("EnablePhoneLogin = %v, want %v", got, tt.want)
			}
		})
	}
}
