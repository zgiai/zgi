package signaturefx

import (
	"os"

	"go.uber.org/fx"

	"plugin_runner/internal/config"
	"plugin_runner/internal/manager"
)

// Module wires a manifest signature verifier if configured.
var Module = fx.Provide(func(cfg *config.Config) manager.SignatureVerifier {
	if !cfg.RequireManifestSignature || cfg.SignaturePublicKeyPath == "" {
		return nil
	}
	data, err := os.ReadFile(cfg.SignaturePublicKeyPath)
	if err != nil {
		return nil
	}
	verifier, err := manager.NewRSAVerifierFromPEM(data)
	if err != nil {
		return nil
	}
	return verifier
})
