package runtimeauth

import "github.com/google/uuid"

// RuntimeAudience describes the caller that wants to use a published runtime
// surface. DepartmentIDs are optional and should be supplied only when a
// department-scoped grant needs to be evaluated.
type RuntimeAudience struct {
	OrganizationID uuid.UUID
	AccountID      uuid.UUID
	DepartmentIDs  []uuid.UUID
	Internal       bool
}

// RuntimeAccessDecisionReason names the first matching allow path or the
// concrete deny reason for a published runtime surface audience check.
type RuntimeAccessDecisionReason string

const (
	RuntimeAccessAllowedOpenSurface       RuntimeAccessDecisionReason = "allowed_open_surface"
	RuntimeAccessAllowedPublicGrant       RuntimeAccessDecisionReason = "allowed_public_grant"
	RuntimeAccessAllowedOrganizationGrant RuntimeAccessDecisionReason = "allowed_organization_grant"
	RuntimeAccessAllowedAccountGrant      RuntimeAccessDecisionReason = "allowed_account_grant"
	RuntimeAccessAllowedDepartmentGrant   RuntimeAccessDecisionReason = "allowed_department_grant"
	RuntimeAccessAllowedInternalGrant     RuntimeAccessDecisionReason = "allowed_internal_grant"
	RuntimeAccessDeniedMissingSurface     RuntimeAccessDecisionReason = "denied_missing_surface"
	RuntimeAccessDeniedDisabledSurface    RuntimeAccessDecisionReason = "denied_disabled_surface"
	RuntimeAccessDeniedNoMatchingGrant    RuntimeAccessDecisionReason = "denied_no_matching_grant"
)

// RuntimeAccessDecision is the explainable form of Allows. Existing callers can
// keep using the boolean helpers while future capability endpoints can expose
// stable states without reimplementing grant matching.
type RuntimeAccessDecision struct {
	Allowed            bool
	Reason             RuntimeAccessDecisionReason
	MatchedSubjectType PublishedRuntimeSubjectType
}

func (auth *ResourceAuthorization) Surface(surface PublishedRuntimeSurface) (SurfaceAuthorization, bool) {
	if auth == nil {
		return SurfaceAuthorization{}, false
	}
	for _, item := range auth.Surfaces {
		if item.Surface == surface {
			return item, true
		}
	}
	return SurfaceAuthorization{}, false
}

// Evaluate checks the requested surface and returns the allow decision with a
// reason that can be mapped to runtime capability states.
func (auth *ResourceAuthorization) Evaluate(surface PublishedRuntimeSurface, audience RuntimeAudience) RuntimeAccessDecision {
	item, ok := auth.Surface(surface)
	if !ok {
		return RuntimeAccessDecision{Reason: RuntimeAccessDeniedMissingSurface}
	}
	return item.Evaluate(audience)
}

func (auth *ResourceAuthorization) Allows(surface PublishedRuntimeSurface, audience RuntimeAudience) bool {
	return auth.Evaluate(surface, audience).Allowed
}

func (auth *ResourceAuthorization) HasGrantType(surface PublishedRuntimeSurface, subjectType PublishedRuntimeSubjectType) bool {
	item, ok := auth.Surface(surface)
	if !ok {
		return false
	}
	for _, grant := range item.Grants {
		if grant.Enabled && grant.SubjectType == subjectType {
			return true
		}
	}
	return false
}

func (surface SurfaceAuthorization) Allows(audience RuntimeAudience) bool {
	return surface.Evaluate(audience).Allowed
}

// Evaluate checks a single surface authorization against the supplied audience.
func (surface SurfaceAuthorization) Evaluate(audience RuntimeAudience) RuntimeAccessDecision {
	if !surface.Enabled {
		return RuntimeAccessDecision{Reason: RuntimeAccessDeniedDisabledSurface}
	}
	if len(surface.Grants) == 0 {
		return RuntimeAccessDecision{Allowed: true, Reason: RuntimeAccessAllowedOpenSurface}
	}

	for _, grant := range surface.Grants {
		if !grant.Enabled {
			continue
		}
		if grantMatchesAudience(grant, audience) {
			return runtimeAccessDecisionForGrant(grant)
		}
	}
	return RuntimeAccessDecision{Reason: RuntimeAccessDeniedNoMatchingGrant}
}

func grantMatchesAudience(grant SurfaceGrant, audience RuntimeAudience) bool {
	switch grant.SubjectType {
	case PublishedRuntimeSubjectPublic:
		return true
	case PublishedRuntimeSubjectInternal:
		return audience.Internal
	case PublishedRuntimeSubjectOrganization:
		if grant.SubjectID == nil {
			return audience.OrganizationID != uuid.Nil
		}
		return audience.OrganizationID == *grant.SubjectID
	case PublishedRuntimeSubjectAccount:
		return grant.SubjectID != nil && audience.AccountID == *grant.SubjectID
	case PublishedRuntimeSubjectDepartment:
		if grant.SubjectID == nil {
			return false
		}
		for _, departmentID := range audience.DepartmentIDs {
			if departmentID == *grant.SubjectID {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func runtimeAccessDecisionForGrant(grant SurfaceGrant) RuntimeAccessDecision {
	decision := RuntimeAccessDecision{
		Allowed:            true,
		MatchedSubjectType: grant.SubjectType,
	}
	switch grant.SubjectType {
	case PublishedRuntimeSubjectPublic:
		decision.Reason = RuntimeAccessAllowedPublicGrant
	case PublishedRuntimeSubjectOrganization:
		decision.Reason = RuntimeAccessAllowedOrganizationGrant
	case PublishedRuntimeSubjectAccount:
		decision.Reason = RuntimeAccessAllowedAccountGrant
	case PublishedRuntimeSubjectDepartment:
		decision.Reason = RuntimeAccessAllowedDepartmentGrant
	case PublishedRuntimeSubjectInternal:
		decision.Reason = RuntimeAccessAllowedInternalGrant
	default:
		decision.Allowed = false
		decision.Reason = RuntimeAccessDeniedNoMatchingGrant
		decision.MatchedSubjectType = ""
	}
	return decision
}
