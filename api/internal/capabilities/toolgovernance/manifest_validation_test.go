package toolgovernance

import (
	"strings"
	"testing"
)

func TestValidateManifestNormalizesValidManifest(t *testing.T) {
	manifest, err := ValidateManifest(Manifest{
		ToolID:                  " file.read ",
		SkillID:                 " file-reader ",
		Domain:                  " Files ",
		Effect:                  " READ ",
		AssetType:               " File ",
		RiskLevel:               " LOW ",
		RequiresAssetResolution: true,
		PermissionScopes:        []string{" File:Read ", "file:read"},
		DefaultApprovalPolicy:   " AUTO_BY_PERMISSION_TIER ",
		AllowedPermissionTiers:  []PermissionTier{" BASIC ", "advanced"},
		AuditRequired:           true,
	})
	if err != nil {
		t.Fatalf("ValidateManifest() error = %v", err)
	}
	if manifest.ToolID != "file.read" || manifest.SkillID != "file-reader" || manifest.Domain != "files" {
		t.Fatalf("manifest identity not normalized: %#v", manifest)
	}
	if manifest.Effect != EffectRead || manifest.AssetType != "file" || manifest.RiskLevel != RiskLevelLow {
		t.Fatalf("manifest fields not normalized: %#v", manifest)
	}
	if len(manifest.PermissionScopes) != 1 || manifest.PermissionScopes[0] != "file:read" {
		t.Fatalf("permission_scopes = %#v, want deduped file:read", manifest.PermissionScopes)
	}
	if len(manifest.AllowedPermissionTiers) != 2 || manifest.AllowedPermissionTiers[0] != PermissionTierBasic || manifest.AllowedPermissionTiers[1] != PermissionTierAdvanced {
		t.Fatalf("allowed_permission_tiers = %#v, want basic/advanced", manifest.AllowedPermissionTiers)
	}
}

func TestValidateManifestRejectsMissingRequiredFields(t *testing.T) {
	_, err := ValidateManifest(Manifest{
		Effect:                 EffectRead,
		RiskLevel:              RiskLevelLow,
		DefaultApprovalPolicy:  ApprovalPolicyAutoByPermissionTier,
		AllowedPermissionTiers: []PermissionTier{PermissionTierBasic},
	})
	assertManifestValidationError(t, err, "tool_id", "skill_id", "domain", "asset_type", "permission_scopes")
}

func TestValidateManifestRejectsInvalidEnums(t *testing.T) {
	_, err := ValidateManifest(Manifest{
		ToolID:                 "file.read",
		SkillID:                "file-reader",
		Domain:                 "files",
		Effect:                 "reed",
		AssetType:              "file",
		RiskLevel:              "hgh",
		PermissionScopes:       []string{"file:read"},
		DefaultApprovalPolicy:  "always",
		AllowedPermissionTiers: []PermissionTier{"superuser"},
	})
	assertManifestValidationError(t, err, "effect", "risk_level", "default_approval_policy", "allowed_permission_tiers")
}

func TestValidateManifestRejectsNoneEffect(t *testing.T) {
	_, err := ValidateManifest(Manifest{
		ToolID:                 "file.none",
		SkillID:                "file-reader",
		Domain:                 "files",
		Effect:                 EffectNone,
		AssetType:              "file",
		RiskLevel:              RiskLevelLow,
		PermissionScopes:       []string{"file:read"},
		DefaultApprovalPolicy:  ApprovalPolicyAutoByPermissionTier,
		AllowedPermissionTiers: []PermissionTier{PermissionTierBasic},
	})
	assertManifestValidationError(t, err, "effect")
}

func assertManifestValidationError(t *testing.T, err error, fields ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("ValidateManifest() error = nil, want validation error containing %v", fields)
	}
	message := err.Error()
	for _, field := range fields {
		if !strings.Contains(message, field) {
			t.Fatalf("ValidateManifest() error = %q, want field %q", message, field)
		}
	}
}
