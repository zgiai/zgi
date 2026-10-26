package catalog

import "slices"

const ZGIMetadataSchemaVersion13 = "1.3.0"

func (d Definition) IsDigitalHumanContract() bool {
	return d.ExtensionSchemaVersion == ZGIMetadataSchemaVersion13 && d.ContractVersion == 4
}

// credentialBase retains the pre-V4 authentication dialect. The extension
// adds a host surface without changing the credential contract.
func credentialBase(d Definition) Definition {
	if !d.IsDigitalHumanContract() {
		return d
	}
	switch d.SetupMethod {
	case SetupMethodAppCredentials:
		d.ExtensionSchemaVersion, d.ContractVersion = ZGIMetadataSchemaVersion11, 2
	case SetupMethodCredentials:
		d.ExtensionSchemaVersion, d.ContractVersion = ZGIMetadataSchemaVersion12, 3
	case SetupMethodSSHCredentials:
		// The SSH credential dialect is native to the V4 contract. It has no
		// earlier published base to project onto, so its version identity is
		// preserved exactly for the digest and compatibility checks.
	default:
		d.ExtensionSchemaVersion, d.ContractVersion = ZGIMetadataSchemaVersion1, 0
	}
	return d
}

// hasAdditiveDigitalHumanSurfaces reports whether values is the four-surface
// additive form: the complete first-release set plus exactly one Digital Human
// surface. Duplicate, unknown, and partial mixed sets are rejected.
func hasAdditiveDigitalHumanSurfaces(values []Surface) bool {
	if len(values) != 4 || !slices.Contains(values, SurfaceDigitalHuman) {
		return false
	}
	base := make([]Surface, 0, 3)
	for _, surface := range values {
		if surface != SurfaceDigitalHuman {
			base = append(base, surface)
		}
	}
	return hasAllFirstReleaseSurfaces(base)
}

func validDefinitionSurfaces(d Definition) bool {
	// Mail retains its V5/V6 credential dialect when the host adds Digital
	// Human. The surface projection must not recast mail as the unrelated V4
	// credential contract.
	if d.IsMailCredentials() {
		return hasAllFirstReleaseSurfaces(d.Surfaces) || hasAdditiveDigitalHumanSurfaces(d.Surfaces)
	}
	if !d.IsDigitalHumanContract() {
		return hasAllFirstReleaseSurfaces(d.Surfaces)
	}
	// A V4 Digital Human contract may be admitted as Digital-Human-only. The
	// additive four-surface form is retained for already accepted contracts.
	if len(d.Surfaces) == 1 && d.Surfaces[0] == SurfaceDigitalHuman {
		return true
	}
	return hasAdditiveDigitalHumanSurfaces(d.Surfaces)
}

// WithDigitalHumanSurface creates a separately admitted contract. Callers must
// persist/activate it through catalog version admission before exposing it.
func WithDigitalHumanSurface(d Definition, pluginVersion string) (Definition, error) {
	if err := Validate(d); err != nil {
		return Definition{}, err
	}
	if pluginVersion == "" || pluginVersion == d.PluginVersion || slices.Contains(d.Surfaces, SurfaceDigitalHuman) {
		return Definition{}, invalid("plugin_version")
	}
	d = cloneDefinition(d)
	if !d.IsMailCredentials() {
		d.ExtensionSchemaVersion, d.ContractVersion = ZGIMetadataSchemaVersion13, 4
	}
	d.PluginVersion, d.PluginDigest = pluginVersion, ""
	if !slices.Contains(d.Surfaces, SurfaceDigitalHuman) {
		d.Surfaces = append(d.Surfaces, SurfaceDigitalHuman)
	}
	return d, Validate(d)
}

func additiveDigitalHumanSurface(current, candidate Definition) bool {
	if slices.Contains(current.Surfaces, SurfaceDigitalHuman) || current.PluginVersion == candidate.PluginVersion {
		return false
	}
	if current.IsMailCredentials() || candidate.IsMailCredentials() {
		return current.IsMailCredentials() && candidate.IsMailCredentials() &&
			current.ExtensionSchemaVersion == candidate.ExtensionSchemaVersion && current.ContractVersion == candidate.ContractVersion &&
			hasAllFirstReleaseSurfaces(current.Surfaces) && hasAdditiveDigitalHumanSurfaces(candidate.Surfaces)
	}
	if !candidate.IsDigitalHumanContract() {
		return false
	}
	base := credentialBase(candidate)
	currentBase := credentialBase(current)
	if currentBase.ExtensionSchemaVersion == "" {
		currentBase.ExtensionSchemaVersion = ZGIMetadataSchemaVersion1
	}
	return base.ExtensionSchemaVersion == currentBase.ExtensionSchemaVersion && hasAdditiveDigitalHumanSurfaces(candidate.Surfaces) && hasAllFirstReleaseSurfaces(current.Surfaces)
}
