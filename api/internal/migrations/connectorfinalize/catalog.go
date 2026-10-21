package connectorfinalize

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
)

// PlanCatalogTransition changes exactly the hidden runtime capability identity.
// It belongs to maintenance, not normal plugin admission or online aliasing.
func PlanCatalogTransition(before catalog.Definition) (catalog.Definition, error) {
	encoded, err := json.Marshal(before)
	if err != nil {
		return catalog.Definition{}, err
	}
	var after catalog.Definition
	if err = json.Unmarshal(encoded, &after); err != nil {
		return after, err
	}
	changed := false
	for i := range after.Operations {
		if after.Operations[i].SkillID == "external-apps" {
			after.Operations[i].SkillID = "connector-runtime"
			changed = true
		}
	}
	if !changed {
		return before, nil
	}
	parts := strings.Split(before.PluginVersion, ".")
	if len(parts) != 3 {
		return after, fmt.Errorf("unsupported plugin maintenance version: %s", before.ID)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil || patch < 0 {
		return after, fmt.Errorf("unsupported plugin maintenance version: %s", before.ID)
	}
	parts[2] = strconv.Itoa(patch + 1)
	after.PluginVersion = strings.Join(parts, ".")
	// Recognize the exact prior embedded manifest, as well as the historical
	// built-in identity digest. Unrecognized filesystem packages require their
	// complete source package and cannot be reconstructed from metadata alone.
	beforeIdentityDigest := digest(struct {
		PluginID      string `json:"plugin_id"`
		PluginVersion string `json:"plugin_version"`
		Dialect       string `json:"source_dialect"`
		SchemaVersion string `json:"schema_version"`
	}{before.PluginID, before.PluginVersion, before.SourceDialect, before.SourceSchemaVersion})
	priorManifest, err := catalog.BuildAgentPluginManifest([]catalog.Definition{before})
	if err != nil {
		return after, err
	}
	priorContentDigest := sha256.Sum256(priorManifest)
	if before.PluginDigest != "" && before.PluginDigest != beforeIdentityDigest && before.PluginDigest != hex.EncodeToString(priorContentDigest[:]) {
		return after, fmt.Errorf("content-backed plugin requires a new admitted package: %s", before.ID)
	}
	manifest, err := catalog.BuildAgentPluginManifest([]catalog.Definition{after})
	if err != nil {
		return after, err
	}
	loader, err := catalog.NewPluginLoader()
	if err != nil {
		return after, err
	}
	loaded, err := loader.LoadManifestPackage(context.Background(), manifest)
	if err != nil || len(loaded.Failures) != 0 || len(loaded.Definitions) != 1 {
		return after, fmt.Errorf("maintenance manifest admission failed")
	}
	after.PluginDigest = loaded.PluginDigest
	expectedContract, expectedErr := catalog.DefinitionContractDigest(after)
	admittedContract, admittedErr := catalog.DefinitionContractDigest(loaded.Definitions[0])
	if expectedErr != nil || admittedErr != nil || expectedContract != admittedContract {
		return after, fmt.Errorf("maintenance manifest changed operation contract")
	}
	if err = validateSkillIdentityTransition(before, after); err != nil {
		return after, err
	}
	return after, catalog.Validate(after)
}

func validateSkillIdentityTransition(before, after catalog.Definition) error {
	if before.ID != after.ID || before.PluginVersion == after.PluginVersion || before.PluginDigest == after.PluginDigest {
		return fmt.Errorf("new immutable plugin version required")
	}
	encoded, err := json.Marshal(after)
	if err != nil {
		return err
	}
	var normalized catalog.Definition
	if err = json.Unmarshal(encoded, &normalized); err != nil {
		return err
	}
	normalized.PluginVersion, normalized.PluginDigest = before.PluginVersion, before.PluginDigest
	if len(before.Operations) != len(normalized.Operations) {
		return fmt.Errorf("maintenance cannot change operation set")
	}
	for i := range before.Operations {
		if before.Operations[i].SkillID == "external-apps" && normalized.Operations[i].SkillID == "connector-runtime" {
			normalized.Operations[i].SkillID = "external-apps"
		}
	}
	if digest(normalized) != digest(before) {
		return fmt.Errorf("maintenance may change only the runtime Skill identity")
	}
	return nil
}
