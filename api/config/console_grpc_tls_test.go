package config

import "testing"

func TestValidateConsoleGRPCTLSAllowsSystemRoots(t *testing.T) {
	if err := ValidateConsoleGRPCTLS(ConsoleGRPCTLSConfig{Enabled: true}); err != nil {
		t.Fatalf("ValidateConsoleGRPCTLS() error = %v", err)
	}
}

func TestValidateConsoleGRPCTLSRejectsUnreadableCA(t *testing.T) {
	err := ValidateConsoleGRPCTLS(ConsoleGRPCTLSConfig{Enabled: true, CAFile: "/missing/console-ca.pem"})
	if err == nil {
		t.Fatal("ValidateConsoleGRPCTLS() accepted an unreadable CA file")
	}
}
