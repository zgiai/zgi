import type { ProviderItem } from '@/services/types/provider';

export type ProviderRuntimeState =
  | 'available_models'
  | 'pending_channels'
  | 'no_catalog_models'
  | 'disabled';

export type ProviderSidebarRuntimeState =
  | ProviderRuntimeState
  | 'configured_no_models'
  | 'unknown';

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

export function getProviderSidebarRuntimeState(
  provider: Pick<ProviderItem, 'is_enabled' | 'model_count' | 'channel_count'>,
  availableModelCount?: number
): ProviderSidebarRuntimeState {
  if (!provider.is_enabled) {
    return 'disabled';
  }

  if ((provider.model_count ?? 0) === 0) {
    return 'no_catalog_models';
  }

  // Availability is the authoritative signal for aliased channel providers such as
  // zhipu -> glm and moonshot -> moonshotai-cn, whose channel_count can be reported as zero.
  if (availableModelCount !== undefined && availableModelCount > 0) {
    return 'available_models';
  }

  if ((provider.channel_count ?? 0) === 0) {
    return 'pending_channels';
  }

  if (availableModelCount === undefined) {
    return 'unknown';
  }

  return 'configured_no_models';
}

export function shouldPromptProviderChannelSetup(
  provider: Pick<ProviderItem, 'is_enabled' | 'model_count' | 'channel_count'>,
  hasAvailableModels: boolean
): boolean {
  return Boolean(
    provider.is_enabled &&
      (provider.model_count ?? 0) > 0 &&
      (provider.channel_count ?? 0) === 0 &&
      !hasAvailableModels
  );
}
