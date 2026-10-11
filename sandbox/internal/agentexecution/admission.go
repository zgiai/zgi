package agentexecution

import (
	"fmt"
	"time"
)

// AdmissionLimits apply to one backend in one control-plane database, not to
// one worker process. Queued Jobs reserve capacity just like running Jobs.
type AdmissionLimits struct {
	Bindings           int `json:"bindings"`
	BindingsPerOrg     int `json:"bindings_per_org"`
	Jobs               int `json:"jobs"`
	JobsPerOrg         int `json:"jobs_per_org"`
	JobsPerBinding     int `json:"jobs_per_binding"`
	JobTimeoutSeconds  int `json:"job_timeout_seconds"`
	BindingsPerEndUser int `json:"bindings_per_end_user,omitempty"`
	JobsPerEndUser     int `json:"jobs_per_end_user,omitempty"`
}

func (l AdmissionLimits) Validate() error {
	if l.BindingsPerEndUser < 0 || l.JobsPerEndUser < 0 {
		return fmt.Errorf("%w: end-user limits cannot be negative", ErrInvalidRequest)
	}
	if l.Bindings < 1 || l.BindingsPerOrg < 1 || l.Jobs < 1 || l.JobsPerOrg < 1 || l.JobsPerBinding < 1 || l.JobTimeoutSeconds < 1 || l.JobTimeoutSeconds > 3600 {
		return fmt.Errorf("%w: positive admission limits and a Job timeout of at most 3600 seconds are required", ErrInvalidRequest)
	}
	if l.BindingsPerOrg > l.Bindings || l.JobsPerOrg > l.Jobs || l.JobsPerBinding > l.JobsPerOrg {
		return fmt.Errorf("%w: admission limits must be ordered by backend, organization and binding", ErrInvalidRequest)
	}
	return nil
}

// WithDefaults keeps historical policy JSON readable without allowing a zero
// value to disable admission for new identity-scoped applications.
func (l AdmissionLimits) WithDefaults() AdmissionLimits {
	if l.BindingsPerEndUser == 0 {
		l.BindingsPerEndUser = 2
	}
	if l.JobsPerEndUser == 0 {
		l.JobsPerEndUser = 1
	}
	return l
}

func (l AdmissionLimits) JobTimeout() time.Duration {
	return time.Duration(l.JobTimeoutSeconds) * time.Second
}

// Uncertain/failed allocations still consume capacity until explicit cleanup.
func BindingConsumesCapacity(status BindingStatus) bool {
	return status != BindingStatusSuspended && status != BindingStatusDeleted
}
