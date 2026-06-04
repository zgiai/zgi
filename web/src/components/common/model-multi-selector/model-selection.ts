import type { ModelItem } from '@/services/types/model';

export type ModelSelectionPolicy = 'available' | 'catalog';

export function buildModelSelectionKey(provider: string | undefined, model: string): string {
  const normalizedProvider = provider?.trim().toLowerCase() || 'unknown';
  return `${normalizedProvider}\t${model.trim()}`;
}

export function getModelSelectionKey(
  model: Pick<ModelItem, 'provider' | 'model'>
): string {
  return buildModelSelectionKey(model.provider, model.model);
}

export function getModelNameFromSelectionKey(key: string): string {
  const separatorIndex = key.indexOf('\t');
  return separatorIndex >= 0 ? key.slice(separatorIndex + 1) : key;
}

export function normalizeSelectableModelKeys(keys: readonly string[]): string[] {
  return keys
    .map(key => {
      const separatorIndex = key.indexOf('\t');
      if (separatorIndex < 0) {
        return buildModelSelectionKey(undefined, key);
      }
      return buildModelSelectionKey(
        key.slice(0, separatorIndex),
        key.slice(separatorIndex + 1)
      );
    })
    .filter(key => getModelNameFromSelectionKey(key).length > 0);
}

export function normalizeSelectableModelNames(names: readonly string[]): string[] {
  return names.map(name => name.trim()).filter(Boolean);
}

export function isModelSelectable(
  model: ModelItem,
  selectionPolicy: ModelSelectionPolicy,
  catalogModelKeys: ReadonlySet<string>,
  selectableModelKeys: ReadonlySet<string> | null,
  selectableModelNames: ReadonlySet<string> | null
): boolean {
  const modelKey = getModelSelectionKey(model);
  const modelName = model.model.trim();

  if (selectableModelNames) {
    if (!selectableModelNames.has(modelName)) {
      return false;
    }
  } else if (selectableModelKeys && !selectableModelKeys.has(modelKey)) {
    return false;
  }

  if (selectionPolicy === 'catalog') {
    return catalogModelKeys.has(modelKey);
  }

  return model.callable !== false && model.is_available !== false;
}
