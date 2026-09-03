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
