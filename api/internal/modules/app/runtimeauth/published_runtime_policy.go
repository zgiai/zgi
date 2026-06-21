package runtimeauth

type PublishedRuntimeSurface string

const (
	PublishedRuntimeSurfaceWebApp     PublishedRuntimeSurface = "webapp"
	PublishedRuntimeSurfaceAPI        PublishedRuntimeSurface = "api"
	PublishedRuntimeSurfaceBuiltinApp PublishedRuntimeSurface = "builtin_app"
	PublishedRuntimeSurfaceInternal   PublishedRuntimeSurface = "internal"
)

const (
	WebAppStatusActive   = "active"
	WebAppStatusInactive = "inactive"
)

type PublishedRuntimePolicy struct {
	WebAppStatus             string
	APIEnabled               bool
	BuiltinAppEnabled        bool
	InternalInvocation       bool
	AllowedBuiltinAccountIDs []string
	AllowedBuiltinDeptIDs    []string
}

func PolicyFromAgentFields(webAppStatus string, apiEnabled bool) PublishedRuntimePolicy {
	normalizedWebAppStatus := NormalizeWebAppStatus(webAppStatus)
	return PublishedRuntimePolicy{
		WebAppStatus:             normalizedWebAppStatus,
		APIEnabled:               apiEnabled,
		InternalInvocation:       true,
		BuiltinAppEnabled:        normalizedWebAppStatus == WebAppStatusActive,
		AllowedBuiltinDeptIDs:    nil,
		AllowedBuiltinAccountIDs: nil,
	}
}

func NormalizeWebAppStatus(status string) string {
	if status == "" {
		return WebAppStatusActive
	}
	return status
}

func (p PublishedRuntimePolicy) Allows(surface PublishedRuntimeSurface) bool {
	switch surface {
	case PublishedRuntimeSurfaceWebApp:
		return NormalizeWebAppStatus(p.WebAppStatus) == WebAppStatusActive
	case PublishedRuntimeSurfaceAPI:
		return p.APIEnabled
	case PublishedRuntimeSurfaceBuiltinApp:
		return p.BuiltinAppEnabled
	case PublishedRuntimeSurfaceInternal:
		return p.InternalInvocation
	default:
		return false
	}
}
