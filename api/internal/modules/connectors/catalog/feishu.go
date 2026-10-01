package catalog

import (
	_ "embed"
	"encoding/json"
)

//go:embed definitions/feishu.json
var feishuDefinitionJSON []byte

//go:embed definitions/feishu-permissions.json
var feishuPermissionsJSON []byte

const FeishuPluginID = "zgi.feishu"

func FeishuDefinition() (Definition, error) {
	var definition Definition
	if err := decodeStrictJSON(feishuDefinitionJSON, &definition); err != nil {
		return Definition{}, err
	}
	if err := Validate(definition); err != nil {
		return Definition{}, err
	}
	var profile map[string]any
	if err := json.Unmarshal(feishuPermissionsJSON, &profile); err != nil {
		return Definition{}, err
	}
	digest, err := digestJSON(profile)
	if err != nil || definition.Setup.TemplateDigest != digest || definition.Setup.TemplateID != profile["profile_id"] {
		return Definition{}, invalid("permission_template_digest")
	}
	return definition, nil
}

// FeishuPermissionTemplate returns a defensive, public configuration snapshot.
// No request-supplied permissions or callback addresses are accepted.
func FeishuPermissionTemplate() map[string]any {
	var profile map[string]any
	_ = json.Unmarshal(feishuPermissionsJSON, &profile)
	return profile
}
