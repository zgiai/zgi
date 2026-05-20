package client

import "errors"

// Client errors
var (
	// ErrAppIDRequired is returned when AppID is empty in AppContext
	ErrAppIDRequired = errors.New("app_id is required")

	// ErrAppTypeRequired is returned when AppType is empty in AppContext
	ErrAppTypeRequired = errors.New("app_type is required")

	// ErrAccountIDRequired is returned when AccountID is empty in AppContext
	ErrAccountIDRequired = errors.New("account_id is required")

	// ErrWorkspaceIDRequired is returned when WorkspaceID is empty in AppContext
	ErrWorkspaceIDRequired = errors.New("workspace_id is required")

	// ErrOrganizationIDRequired is returned when OrganizationID cannot be determined
	ErrOrganizationIDRequired = errors.New("organization_id is required")

	// ErrTenantIDRequired is kept as a compatibility alias.
	ErrTenantIDRequired = ErrOrganizationIDRequired

	// ErrInvalidAppType is returned when AppType is not recognized
	ErrInvalidAppType = errors.New("invalid app_type: must be 'agent', 'dataset', or 'workflow'")

	// ErrSystemKeyCreationFailed is returned when internal system API key creation fails
	ErrSystemKeyCreationFailed = errors.New("failed to create internal system API key")
)
