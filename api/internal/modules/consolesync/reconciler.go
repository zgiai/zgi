package consolesync

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Reconciler performs the light drift check required by plan P6: it compares at most a
// couple of console-controlled fields and reports the differences on the next heartbeat.
// It never rewrites local state; console decides what to do with the drift.
type Reconciler struct {
	store *Store
}

// NewReconciler creates the reconciler.
func NewReconciler(store *Store) *Reconciler {
	return &Reconciler{store: store}
}

// Drift compares the desired state with the local snapshot.
func (r *Reconciler) Drift(desired *DesiredStateResponse, snapshot *ControlledSnapshot) []Drift {
	if desired == nil || snapshot == nil {
		return nil
	}

	var drift []Drift

	// 1. Account status (organizations.status).
	if desiredStatus := strings.TrimSpace(desired.AccountStatus); desiredStatus != "" {
		if snapshot.AccountStatus != desiredStatus {
			drift = append(drift, Drift{
				Scope:    "organization",
				Field:    "status",
				ScopeID:  snapshot.OrganizationID,
				Desired:  desiredStatus,
				Observed: snapshot.AccountStatus,
			})
		}
	}

	// 2. Workspace quota limits, when console published them.
	for workspaceID, limit := range desiredQuotas(desired.Quota) {
		observed, found := snapshot.WorkspaceQuotas[workspaceID]
		if !found {
			drift = append(drift, Drift{
				Scope:    "workspace",
				Field:    "quota_limit",
				ScopeID:  workspaceID,
				Desired:  strconv.FormatInt(limit, 10),
				Observed: "missing",
			})
			continue
		}
		if observed != limit {
			drift = append(drift, Drift{
				Scope:    "workspace",
				Field:    "quota_limit",
				ScopeID:  workspaceID,
				Desired:  strconv.FormatInt(limit, 10),
				Observed: strconv.FormatInt(observed, 10),
			})
		}
	}

	return drift
}

// desiredQuotas decodes the quota projection. The accepted shapes are
// {"workspaces":{"<id>":<limit>}} and {"<id>":<limit>}; anything else is ignored so a
// future console-side extension cannot break the reconcile loop.
func desiredQuotas(raw string) map[string]int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}

	quotas := map[string]int64{}
	var envelope struct {
		Workspaces map[string]json.Number `json:"workspaces"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err == nil && len(envelope.Workspaces) > 0 {
		for id, value := range envelope.Workspaces {
			if limit, err := value.Int64(); err == nil {
				quotas[id] = limit
			}
		}
		return quotas
	}

	var flat map[string]json.Number
	if err := json.Unmarshal([]byte(raw), &flat); err == nil {
		for id, value := range flat {
			if limit, err := value.Int64(); err == nil {
				quotas[id] = limit
			}
		}
	}
	return quotas
}

// Describe renders the drift list for logs.
func Describe(drift []Drift) string {
	if len(drift) == 0 {
		return "no drift"
	}
	parts := make([]string, 0, len(drift))
	for _, item := range drift {
		parts = append(parts, fmt.Sprintf("%s.%s(%s) desired=%s observed=%s",
			item.Scope, item.Field, item.ScopeID, item.Desired, item.Observed))
	}
	return strings.Join(parts, "; ")
}
