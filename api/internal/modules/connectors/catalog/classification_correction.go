package catalog

// These immutable contract digests identify the reviewed web-search metadata
// correction. They intentionally preserve the original V1 digest algorithm and
// every Key, capability, surface, operation, exposure, and risk contract. A
// different historical contract requires a separate review, not a wider rule.
const (
	webSearchDataSourceContractDigest = "15e3d66cd3cff91e5a8d107444a66c623304f41ce15591beceb5275a5bb1571c"
	webSearchConnectorContractDigest  = "01597a8e615f7bd6d7a6d16c92221b57c8c7be4122a7c3d6e93cd3ff44fcc401"
)

func reviewedClassificationCorrection(current, candidate Definition) bool {
	if current.ID != "web-search" || candidate.ID != current.ID ||
		current.PluginID != "zgi.exa-search" || candidate.PluginID != current.PluginID ||
		current.ProviderID != "web-search" || candidate.ProviderID != current.ProviderID ||
		current.DriverID != "exa" || candidate.DriverID != current.DriverID ||
		current.AuthMethodID != "personal_api_key" || candidate.AuthMethodID != current.AuthMethodID ||
		current.SourceDialect != SourceDialectAgentPluginsV1 || candidate.SourceDialect != current.SourceDialect ||
		current.SourceSchemaVersion != SourceSchemaVersion1 || candidate.SourceSchemaVersion != current.SourceSchemaVersion ||
		(current.PluginVersion != "1.0.0" && current.PluginVersion != "1.1.0") || candidate.PluginVersion != "1.2.0" ||
		current.Classification != ClassificationDataSource || candidate.Classification != ClassificationConnector {
		return false
	}
	oldDigest, err := DefinitionContractDigest(current)
	if err != nil || oldDigest != webSearchDataSourceContractDigest {
		return false
	}
	newDigest, err := DefinitionContractDigest(candidate)
	return err == nil && newDigest == webSearchConnectorContractDigest
}
