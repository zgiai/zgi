import type { ModelItem } from '@/services/types/model';

export interface ModelSelectorValue {
  provider: string;
  model: string;
}

// Expose full selected model props including provider for external control
export type ModelSelectorModelProps = ModelItem;

// Feature labels for tooltip display (dynamic keys from API)
export type FeatureLabels = Record<string, string>;

// Provider group structure for display
export interface ProviderGroup {
  provider: string;
  models: ModelItem[];
}

// Flattened row types for virtualization
export type FlatRow =
  | { type: 'header'; providerId: string; providerLabel: string; modelCount: number }
  | { type: 'model'; providerId: string; model: ModelItem };
