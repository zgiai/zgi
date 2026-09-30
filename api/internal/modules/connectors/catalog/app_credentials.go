package catalog

import (
	"slices"
	"sort"
	"strings"
)

const (
	ZGIMetadataSchemaVersion11             = "1.1.0"
	SetupMethodAppCredentials  SetupMethod = "app_credentials"
)

// CredentialField describes an input, never a saved value. Only an explicitly
// versioned app contract may declare non-secret inputs.
type CredentialField struct {
	Name            string        `json:"name"`
	Label           string        `json:"label"`
	LabelI18n       LocalizedText `json:"label_i18n,omitempty"`
	Placeholder     string        `json:"placeholder,omitempty"`
	PlaceholderI18n LocalizedText `json:"placeholder_i18n,omitempty"`
	Required        bool          `json:"required"`
	Secret          bool          `json:"secret"`
	Type            string        `json:"type"`
	Enum            []string      `json:"enum,omitempty"`
	MaxLength       int           `json:"max_length"`
}

// SetupDescriptor selects a platform implementation, not executable plugin code.
type SetupDescriptor struct {
	Handler            string   `json:"handler"`
	Methods            []string `json:"methods"`
	TemplateID         string   `json:"template_id"`
	TemplateDigest     string   `json:"template_digest"`
	ProtocolVersion    int      `json:"protocol_version,omitempty"`
	Intents            []string `json:"intents,omitempty"`
	AuthenticationKind string   `json:"authentication_kind,omitempty"`
	VerificationPolicy string   `json:"verification_policy,omitempty"`
}

func (d Definition) IsAppCredentials() bool {
	return d.ExtensionSchemaVersion == ZGIMetadataSchemaVersion11 || d.IsDigitalHumanContract() && d.SetupMethod == SetupMethodAppCredentials
}

func validateCredentialContract(d Definition) error {
	d = credentialBase(d)
	if d.IsMailCredentials() {
		return validateMailContract(d)
	}
	if d.IsMessagingCredentials() {
		return validateMessagingContract(d)
	}
	if !d.IsAppCredentials() {
		if (d.ExtensionSchemaVersion != "" && d.ExtensionSchemaVersion != ZGIMetadataSchemaVersion1) || d.ContractVersion != 0 || len(d.CredentialFields) != 0 || d.Setup != nil {
			return invalid("extension_schema_version")
		}
		if d.SetupMethod != SetupMethodKey {
			return invalid("setup_method")
		}
		for _, op := range d.Operations {
			if op.CredentialSubject != "" || len(op.RequiredScopes) != 0 {
				return invalid("operation_permissions")
			}
		}
		return validateKeyFields(d.KeyFields)
	}
	if d.ContractVersion != 2 || d.SetupMethod != SetupMethodAppCredentials || len(d.KeyFields) != 0 || len(d.CredentialFields) != 3 {
		return invalid("credential_contract")
	}
	if d.DriverID != "feishu-openapi-v1" || d.ProviderID != "feishu" || d.AuthMethodID != "feishu.app_credentials" {
		return invalid("credential_driver")
	}
	seen := map[string]bool{}
	for _, field := range d.CredentialFields {
		if seen[field.Name] || !field.Required || field.Type != "string" || strings.TrimSpace(field.Label) == "" || !validLocalizedText(field.LabelI18n) || !validLocalizedText(field.PlaceholderI18n) {
			return invalid("credential_fields")
		}
		seen[field.Name] = true
		switch field.Name {
		case "region":
			if field.Secret || field.MaxLength != 6 || len(field.Enum) != 2 || !slices.Contains(field.Enum, "feishu") || !slices.Contains(field.Enum, "lark") {
				return invalid("credential_region")
			}
		case "app_id":
			if field.Secret || field.MaxLength != 128 || len(field.Enum) != 0 {
				return invalid("credential_app_id")
			}
		case "app_secret":
			if !field.Secret || field.MaxLength != 16384 || len(field.Enum) != 0 {
				return invalid("credential_secret")
			}
		default:
			return invalid("credential_fields")
		}
	}
	if d.Setup == nil || d.Setup.ProtocolVersion != 0 || len(d.Setup.Intents) != 0 || d.Setup.AuthenticationKind != "" || d.Setup.VerificationPolicy != "" || d.Setup.Handler != "feishu-app-registration-v1" || !stableIdentifier(d.Setup.TemplateID) || !sha256HexPattern.MatchString(d.Setup.TemplateDigest) || len(d.Setup.Methods) != 3 {
		return invalid("setup_descriptor")
	}
	for _, method := range []string{"manual", "qr_create", "qr_update"} {
		if !slices.Contains(d.Setup.Methods, method) {
			return invalid("setup_methods")
		}
	}
	for _, op := range d.Operations {
		if op.CredentialSubject != "app" || len(op.RequiredScopes) == 0 || len(op.RequiredScopes) > 16 {
			return invalid("operation_permissions")
		}
		for _, alternatives := range op.RequiredScopes {
			if len(alternatives) == 0 || len(alternatives) > 16 {
				return invalid("operation_permissions")
			}
			unique := map[string]bool{}
			for _, scope := range alternatives {
				if scope == "" || len(scope) > 128 || strings.TrimSpace(scope) != scope || strings.ContainsAny(scope, "\r\n\t /\\") || unique[scope] {
					return invalid("operation_permissions")
				}
				unique[scope] = true
			}
		}
	}
	return nil
}

// ValidateCredentialValues checks the complete admitted schema without
// retaining any submitted values. Legacy Key validation remains unchanged.
func ValidateCredentialValues(d Definition, values map[string]string) error {
	if err := validateCredentialContract(d); err != nil {
		return err
	}
	if d.IsMailCredentials() {
		copy := make(map[string]string, len(values))
		for k, v := range values {
			copy[k] = v
		}
		defer clear(copy)
		return NormalizeMailValuesFor(d.ID, copy)
	}
	if d.UsesCredentials() {
		if len(values) != len(d.CredentialFields) {
			return invalid("credential_values")
		}
		for _, field := range d.CredentialFields {
			v, exists := values[field.Name]
			if !exists || strings.TrimSpace(v) == "" || len(v) > field.MaxLength || strings.ContainsAny(v, "\r\n\x00") {
				return invalid("credential_values")
			}
			if len(field.Enum) > 0 && !slices.Contains(field.Enum, v) {
				return invalid("credential_values")
			}
			if d.IsMessagingCredentials() && field.Name == "base_url" && !validMessagingOrigin(v) {
				return invalid("credential_origin")
			}
			if field.Name == "app_id" {
				for _, c := range v {
					if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
						return invalid("credential_values")
					}
				}
			}
		}
		return nil
	}
	if len(values) == 0 || len(values) > len(d.KeyFields) {
		return invalid("credential_values")
	}
	allowed := map[string]KeyField{}
	for _, f := range d.KeyFields {
		allowed[f.Name] = f
		if f.Required && strings.TrimSpace(values[f.Name]) == "" {
			return invalid("credential_values")
		}
	}
	for k, v := range values {
		if f, ok := allowed[k]; !ok || !f.Secret || strings.TrimSpace(v) == "" || len(v) > 16384 {
			return invalid("credential_values")
		}
	}
	return nil
}

func cloneCredentialFields(fields []CredentialField) []CredentialField {
	if fields == nil {
		return nil
	}
	cloned := append([]CredentialField(nil), fields...)
	for i, f := range fields {
		cloned[i].Enum = slices.Clone(f.Enum)
		cloned[i].LabelI18n = f.LabelI18n.Clone()
		cloned[i].PlaceholderI18n = f.PlaceholderI18n.Clone()
	}
	return cloned
}

func cloneSetup(setup *SetupDescriptor) *SetupDescriptor {
	if setup == nil {
		return nil
	}
	cloned := *setup
	cloned.Methods = slices.Clone(setup.Methods)
	cloned.Intents = slices.Clone(setup.Intents)
	return &cloned
}

func cloneScopes(groups [][]string) [][]string {
	if groups == nil {
		return nil
	}
	cloned := make([][]string, len(groups))
	for i, group := range groups {
		cloned[i] = slices.Clone(group)
	}
	return cloned
}

type frozenCredentialFieldV2 struct {
	Name      string   `json:"name"`
	Required  bool     `json:"required"`
	Secret    bool     `json:"secret"`
	Type      string   `json:"type"`
	Enum      []string `json:"enum,omitempty"`
	MaxLength int      `json:"max_length"`
}
type frozenAuthenticationV2 struct {
	ExtensionSchemaVersion string                    `json:"extension_schema_version"`
	ContractVersion        int                       `json:"contract_version"`
	Fields                 []frozenCredentialFieldV2 `json:"credential_fields"`
	Setup                  *SetupDescriptor          `json:"setup"`
}
type frozenPermissionV2 struct {
	OperationID       string     `json:"operation_id"`
	CredentialSubject string     `json:"credential_subject"`
	RequiredScopes    [][]string `json:"required_scopes"`
}

func canonicalAuthentication(d Definition) frozenAuthenticationV2 {
	fields := make([]frozenCredentialFieldV2, 0, len(d.CredentialFields))
	for _, f := range d.CredentialFields {
		values := slices.Clone(f.Enum)
		sort.Strings(values)
		fields = append(fields, frozenCredentialFieldV2{f.Name, f.Required, f.Secret, f.Type, values, f.MaxLength})
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	setup := cloneSetup(d.Setup)
	if setup != nil {
		sort.Strings(setup.Methods)
	}
	return frozenAuthenticationV2{d.ExtensionSchemaVersion, d.ContractVersion, fields, setup}
}
func canonicalPermissions(op Operation) frozenPermissionV2 {
	groups := cloneScopes(op.RequiredScopes)
	for _, group := range groups {
		sort.Strings(group)
	}
	sort.Slice(groups, func(i, j int) bool { return strings.Join(groups[i], "\x00") < strings.Join(groups[j], "\x00") })
	return frozenPermissionV2{op.ID, op.CredentialSubject, groups}
}
func canonicalDefinitionContractV2(d Definition) any {
	permissions := make([]frozenPermissionV2, 0, len(d.Operations))
	for _, op := range d.Operations {
		permissions = append(permissions, canonicalPermissions(op))
	}
	sort.Slice(permissions, func(i, j int) bool { return permissions[i].OperationID < permissions[j].OperationID })
	return struct {
		Base           frozenDefinitionContractV1 `json:"base"`
		Authentication frozenAuthenticationV2     `json:"authentication"`
		Permissions    []frozenPermissionV2       `json:"permissions"`
	}{canonicalDefinitionContract(d), canonicalAuthentication(d), permissions}
}
