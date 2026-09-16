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
