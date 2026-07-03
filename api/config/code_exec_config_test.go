package config

import "testing"

func TestLoadCodeExecConfigLoadsAdapterTimeouts(t *testing.T) {
	cfg := &Config{}
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		switch key {
		case envCodeExecutionEndpoint:
			return "http://sandbox.local", true
		case envCodeExecutionAPIKey:
			return "sandbox-key", true
		case envCodeExecutionConnectTimeout:
			return "2", true
		case envCodeExecutionCreateTimeout:
			return "3", true
		case envCodeExecutionUploadTimeout:
			return "4", true
		case envCodeExecutionCommandTimeoutPadding:
			return "5", true
		case envCodeExecutionArtifactTimeout:
			return "6", true
		case envCodeExecutionCleanupTimeout:
			return "7", true
		case envCodeExecutionEnableNetwork:
			return "true", true
		case envCodeExecutionSystemOfficeProfile:
			return "system-office", true
		default:
			return "", false
		}
	}}

	if err := loadCodeExecConfig(cfg, source); err != nil {
		t.Fatalf("loadCodeExecConfig() error = %v", err)
	}
	if cfg.CodeExec.Endpoint != "http://sandbox.local" || cfg.CodeExec.APIKey != "sandbox-key" {
		t.Fatalf("unexpected code execution endpoint or key: %+v", cfg.CodeExec)
	}
	if cfg.CodeExec.ConnectTimeoutSeconds != 2 ||
		cfg.CodeExec.CreateTimeoutSeconds != 3 ||
		cfg.CodeExec.UploadTimeoutSeconds != 4 ||
		cfg.CodeExec.CommandTimeoutPaddingSeconds != 5 ||
		cfg.CodeExec.ArtifactTimeoutSeconds != 6 ||
		cfg.CodeExec.CleanupTimeoutSeconds != 7 ||
		!cfg.CodeExec.EnableNetwork ||
		cfg.CodeExec.SystemOfficeProfile != "system-office" {
		t.Fatalf("unexpected code execution timeouts: %+v", cfg.CodeExec)
	}
}

func TestLoadCodeExecConfigUsesAdapterTimeoutDefaults(t *testing.T) {
	cfg := &Config{}
	source := &envSource{lookupEnv: func(string) (string, bool) { return "", false }}

	if err := loadCodeExecConfig(cfg, source); err != nil {
		t.Fatalf("loadCodeExecConfig() error = %v", err)
	}
	if cfg.CodeExec.ConnectTimeoutSeconds != 5 ||
		cfg.CodeExec.CreateTimeoutSeconds != 10 ||
		cfg.CodeExec.UploadTimeoutSeconds != 30 ||
		cfg.CodeExec.CommandTimeoutPaddingSeconds != 15 ||
		cfg.CodeExec.ArtifactTimeoutSeconds != 10 ||
		cfg.CodeExec.CleanupTimeoutSeconds != 5 ||
		cfg.CodeExec.EnableNetwork ||
		cfg.CodeExec.SystemOfficeProfile != "skill-office" {
		t.Fatalf("unexpected default code execution timeouts: %+v", cfg.CodeExec)
	}
}
