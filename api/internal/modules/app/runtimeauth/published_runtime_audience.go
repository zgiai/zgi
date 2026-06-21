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

func (auth *ResourceAuthorization) Allows(surface PublishedRuntimeSurface, audience RuntimeAudience) bool {
	item, ok := auth.Surface(surface)
	if !ok {
		return false
	}
	return item.Allows(audience)
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
	if !surface.Enabled {
		return false
	}
	if len(surface.Grants) == 0 {
		return true
	}

	for _, grant := range surface.Grants {
		if !grant.Enabled {
			continue
		}
		if grantMatchesAudience(grant, audience) {
			return true
		}
	}
	return false
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
