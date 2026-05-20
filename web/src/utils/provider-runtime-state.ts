import type { ProviderItem } from '@/services/types/provider';

export type ProviderRuntimeState =
  | 'available_models'
  | 'pending_channels'
  | 'no_catalog_models'
  | 'disabled';

export function getProviderRuntimeState(
  provider: Pick<ProviderItem, 'is_enabled' | 'model_count'>,
  availableModelCount: number
): ProviderRuntimeState {
  if (!provider.is_enabled) {
    return 'disabled';
  }

  if (availableModelCount > 0) {
    return 'available_models';
  }

  if ((provider.model_count ?? 0) > 0) {
    return 'pending_channels';
  }

  return 'no_catalog_models';
}
