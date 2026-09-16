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
	default:
		d.ExtensionSchemaVersion, d.ContractVersion = ZGIMetadataSchemaVersion1, 0
	}
	return d
}

func validDefinitionSurfaces(d Definition) bool {
	if !d.IsDigitalHumanContract() {
		return hasAllFirstReleaseSurfaces(d.Surfaces)
	}
	if len(d.Surfaces) != 4 || !slices.Contains(d.Surfaces, SurfaceDigitalHuman) {
		return false
	}
	base := make([]Surface, 0, 3)
	for _, surface := range d.Surfaces {
		if surface != SurfaceDigitalHuman {
			base = append(base, surface)
		}
	}
	return hasAllFirstReleaseSurfaces(base)
}

// WithDigitalHumanSurface creates a separately admitted contract. Callers must
// persist/activate it through catalog version admission before exposing it.
func WithDigitalHumanSurface(d Definition, pluginVersion string) (Definition, error) {
	if err := Validate(d); err != nil {
		return Definition{}, err
	}
	if pluginVersion == "" || pluginVersion == d.PluginVersion {
		return Definition{}, invalid("plugin_version")
	}
	d = cloneDefinition(d)
	d.ExtensionSchemaVersion, d.ContractVersion = ZGIMetadataSchemaVersion13, 4
	d.PluginVersion, d.PluginDigest = pluginVersion, ""
	if !slices.Contains(d.Surfaces, SurfaceDigitalHuman) {
		d.Surfaces = append(d.Surfaces, SurfaceDigitalHuman)
	}
	return d, Validate(d)
}

func additiveDigitalHumanSurface(current, candidate Definition) bool {
	if current.IsDigitalHumanContract() || !candidate.IsDigitalHumanContract() || current.PluginVersion == candidate.PluginVersion {
		return false
	}
	base := credentialBase(candidate)
	currentBase := credentialBase(current)
	if currentBase.ExtensionSchemaVersion == "" {
		currentBase.ExtensionSchemaVersion = ZGIMetadataSchemaVersion1
	}
	return base.ExtensionSchemaVersion == currentBase.ExtensionSchemaVersion && validDefinitionSurfaces(candidate) && hasAllFirstReleaseSurfaces(current.Surfaces)
}
