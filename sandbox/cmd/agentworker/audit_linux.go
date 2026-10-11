//go:build linux

package main

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	e2b "github.com/zgiai/zgi-sandbox/internal/hostedruntime/e2b"
	"github.com/zgiai/zgi-sandbox/internal/localruntime/containerd"
	"github.com/zgiai/zgi-sandbox/internal/storage"
)

func auditRuntime(parent context.Context, cfg workerConfig) error {
	report, err := collectRuntimeAudit(parent, cfg)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(report)
}

func auditMetrics(parent context.Context, cfg workerConfig) error {
	report, err := collectRuntimeAudit(parent, cfg)
	if err != nil {
		return err
	}
	return agentexecution.WriteAuditMetrics(os.Stdout, report)
}

func collectRuntimeAudit(parent context.Context, cfg workerConfig) (agentexecution.RuntimeAudit, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	var resources []agentexecution.ResourceObservation
	var err error
	if cfg.Backend == localBackendName {
		resources, err = containerd.ReadInventory(ctx, cfg.ContainerdAddress)
	} else {
		backend, createErr := e2b.New(e2b.Options{APIKey: cfg.E2BAPIKey, APIURL: cfg.E2BAPIURL, Domain: cfg.E2BDomain, Profiles: cfg.E2BProfiles, EgressProxy: e2bEgressProxyConfig(cfg)})
		if createErr != nil {
			return agentexecution.RuntimeAudit{}, createErr
		}
		resources, err = backend.Inventory(ctx)
	}
	if err != nil {
		return agentexecution.RuntimeAudit{}, err
	}
	return storage.ReadRuntimeAudit(ctx, cfg.DatabaseURL, cfg.Backend, resources)
}
