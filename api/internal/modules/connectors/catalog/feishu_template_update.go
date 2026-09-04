package catalog

// Only this reviewed release removes card-reading from the registration
// template. These exact full contracts preserve credentials, surfaces, native
// operations and risks. Arbitrary template/authentication changes remain
// incompatible; accepting this metadata never modifies an external app.
func reviewedFeishuTemplateReduction(current, candidate Definition) bool {
	if current.ID != "feishu" || candidate.ID != current.ID ||
		current.PluginID != FeishuPluginID || candidate.PluginID != current.PluginID ||
		current.PluginVersion != "1.0.0" || candidate.PluginVersion != "1.0.1" {
		return false
	}
	oldDigest, err := DefinitionContractDigest(current)
	if err != nil || oldDigest != "ba57e8d4e04ae53e79c7cbd24e9c0d36dedbe32594cdd39d55bf6cba3da77de8" {
		return false
	}
	newDigest, err := DefinitionContractDigest(candidate)
	return err == nil && newDigest == "e3210d6530df705bd67ac11ebd0b84abd03f9afe3358b77247e13121eb14ca5e"
}

// This one-time v2 -> v3 transition pins the complete reviewed authentication
// contracts, not an unrelated plugin release or operation set. These digests
// were frozen from the accepted 1.0.2 authentication and the stable v3 template;
// do not derive them from the currently loaded manifest. Normal compatibility
// checks still protect identities, surfaces, and every existing operation.
// Catalog admission never changes credentials, QR receipts, or external grants.
func reviewedFeishuTemplateExpansion(current, candidate Definition) bool {
	if current.ID != "feishu" || candidate.ID != current.ID ||
		current.PluginID != FeishuPluginID || candidate.PluginID != current.PluginID ||
		current.PluginVersion == candidate.PluginVersion {
		return false
	}
	oldDigest, err := digestJSON(canonicalAuthentication(current))
	if err != nil || oldDigest != "e8eb91b5a336036bea786350d1062036efe15faf9c933966c4674c4159fc8063" {
		return false
	}
	newDigest, err := digestJSON(canonicalAuthentication(candidate))
	return err == nil && newDigest == "fe5d0bc4b8c79a75eb673839e88a6b4de9ab65582b876228bdb95463ed0b74c4"
}
