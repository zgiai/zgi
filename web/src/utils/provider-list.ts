import type { ProviderItem } from '@/services/types/provider';

export interface ProviderGroups {
  officialProviders: ProviderItem[];
  customProviders: ProviderItem[];
}

export function partitionProviders(providers: readonly ProviderItem[]): ProviderGroups {
  const officialProviders: ProviderItem[] = [];
  const customProviders: ProviderItem[] = [];

  for (const provider of providers) {
    switch (provider.provider_type) {
      case 'global':
        officialProviders.push(provider);
        break;
      case 'custom':
        customProviders.push(provider);
        break;
      default:
        throw new Error(`Unsupported provider type: ${String(provider.provider_type)}`);
    }
  }

  return { officialProviders, customProviders };
}
