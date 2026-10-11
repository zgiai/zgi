package skills

// Script package limits are shared by custom-skill import validation and the
// execution snapshot so an accepted package remains executable.
const (
	ScriptPackageMaxBytes     = 64 * 1024 * 1024
	ScriptPackageMaxFileBytes = 10 * 1024 * 1024
	ScriptPackageMaxFiles     = 600
)
