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

// The v3 template adds the reviewed tenant/message/member/directory scopes to
// QR setup and one bounded name-search operation. The subsequent 1.2.0 release
// adds only the reviewed tenant and chat-member reads. Existing operations stay exact.
// Pin both complete contracts and releases: this must not allow arbitrary
// permission expansion, runtime changes, or rewrites of existing QR receipts.
// Catalog admission only updates local metadata, never the external app grants.
func reviewedFeishuTemplateExpansion(current, candidate Definition) bool {
	if current.ID != "feishu" || candidate.ID != current.ID ||
		current.PluginID != FeishuPluginID || candidate.PluginID != current.PluginID ||
		current.PluginVersion != "1.0.2" {
		return false
	}
	oldDigest, err := DefinitionContractDigest(current)
	if err != nil || oldDigest != "8313044f4f770570c1e567ad24af7f30b7b497ae6fa8b19b2eacf3d8a405c430" {
		return false
	}
	newDigest, err := DefinitionContractDigest(candidate)
	if err != nil {
		return false
	}
	switch candidate.PluginVersion {
	case "1.1.0":
		return newDigest == "66d7eea4a48346671b7756a11be144fbb5704c60e82eb9557cb26f731a19ff43"
	case "1.2.0":
		return newDigest == "d9db772907e21bf034a8ba78ea6a84f1fa829de397c5c0b0284996968452e2e3"
	default:
		return false
	}
}
