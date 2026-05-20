package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/zgiai/ginext/config"
)

// CloudBootstrapRunner loads cloud bootstrap input and runs first-time initialization.
type CloudBootstrapRunner struct {
	cfg              *config.Config
	bootstrapService *BootstrapService
}

// NewCloudBootstrapRunner creates a cloud bootstrap runner.
func NewCloudBootstrapRunner(cfg *config.Config, bootstrapService *BootstrapService) *CloudBootstrapRunner {
	return &CloudBootstrapRunner{
		cfg:              cfg,
		bootstrapService: bootstrapService,
	}
}

// Run performs bootstrap only when cloud mode still has no setup marker.
func (r *CloudBootstrapRunner) Run(ctx context.Context) error {
	if r == nil || r.cfg == nil || r.bootstrapService == nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(r.cfg.Platform.Edition), "CLOUD") {
		return nil
	}

	setupStatus, err := r.bootstrapService.GetSetupStatus()
	if err != nil {
		return fmt.Errorf("get setup status: %w", err)
	}
	if setupStatus != nil {
		return nil
	}

	params, err := r.bootstrapParams()
	if err != nil {
		return err
	}

	if err := r.bootstrapService.Bootstrap(ctx, params); err != nil {
		return fmt.Errorf("run cloud bootstrap: %w", err)
	}

	return nil
}

func (r *CloudBootstrapRunner) bootstrapParams() (BootstrapParams, error) {
	cfg := r.cfg.Platform.CloudBootstrap
	if strings.TrimSpace(cfg.AdminEmail) == "" || strings.TrimSpace(cfg.AdminName) == "" || strings.TrimSpace(cfg.AdminPassword) == "" {
		return BootstrapParams{}, ErrCloudBootstrapConfig
	}

	return BootstrapParams{
		AdminEmail:    strings.TrimSpace(cfg.AdminEmail),
		AdminName:     strings.TrimSpace(cfg.AdminName),
		AdminPassword: cfg.AdminPassword,
		Source:        BootstrapSourceCloudEnv,
	}, nil
}
