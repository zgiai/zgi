import type { ModelSelectorModelProps } from '@/components/common/model-selector';

export interface AIChatModelIdentity {
  provider?: string | null;
  model?: string | null;
}

export function findAIChatModelProps(
  models: ModelSelectorModelProps[],
  identity: AIChatModelIdentity
): ModelSelectorModelProps | null {
  const provider = identity.provider?.trim();
  const model = identity.model?.trim();
  if (!provider || !model) {
    return null;
  }
  return (
    models.find(item => item.provider === provider && item.model === model) ??
    models.find(item => item.provider === provider && item.model_name === model) ??
    null
  );
}
