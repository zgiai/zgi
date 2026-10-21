package config

import "testing"

func TestValidateConsoleAPIURLRequiresHTTPSOutsideDevelopment(t *testing.T) {
	cfg := developmentTestConfig()
	cfg.Server.Mode = "release"
	cfg.Server.Environment = "production"
	cfg.Console.APIURL = "http://console.example.com"

	if err := validateConsoleAPIURL(cfg); err == nil {
		t.Fatal("validateConsoleAPIURL() accepted HTTP in production")
	}
}

func TestValidateConsoleAPIURLAllowsHTTPInDevelopment(t *testing.T) {
	cfg := developmentTestConfig()
	cfg.Console.APIURL = "http://127.0.0.1:2670"

	if err := validateConsoleAPIURL(cfg); err != nil {
		t.Fatalf("validateConsoleAPIURL() rejected development HTTP: %v", err)
	}
}

func TestValidateConsoleAPIURLRejectsPathPrefix(t *testing.T) {
	cfg := developmentTestConfig()
	cfg.Console.APIURL = "https://console.example.com/v1"

	if err := validateConsoleAPIURL(cfg); err == nil {
		t.Fatal("validateConsoleAPIURL() accepted a path prefix")
	}
}
