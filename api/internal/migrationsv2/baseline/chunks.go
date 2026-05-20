package baseline

type Chunk struct {
	Name string
	File string
}

func Chunks() []Chunk {
	return []Chunk{
		baselineIdentityWorkspaceChunk(),
		baselineCatalogSystemChunk(),
		baselineBillingLLMChunk(),
		baselineFileDatasetGraphflowChunk(),
		baselineAgentWorkflowChunk(),
		baselineAutomationChunk(),
		baselineCompatibilityViewsChunk(),
	}
}
