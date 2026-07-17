import type { ModelItem, ModelUseCase } from '@/services/types/model';
import type { ModelSelectorValue } from './types';

// Use compact URI-encoded value encoding to avoid JSON stringify/parse overhead
const SEP = '::';

export const serializeValue = (v: ModelSelectorValue): string =>
  `${encodeURIComponent(v.provider)}${SEP}${encodeURIComponent(v.model)}`;

export const deserializeValue = (s: string): ModelSelectorValue | null => {
  const [p, m] = s.split(SEP);
  if (p === undefined || m === undefined) return null;
  return { provider: decodeURIComponent(p), model: decodeURIComponent(m) };
};

export function prioritizeModelsByUseCase(
  models: ModelItem[],
  preferredUseCase?: ModelUseCase
): ModelItem[] {
  if (!preferredUseCase) return models;

  const preferred: ModelItem[] = [];
  const remaining: ModelItem[] = [];
  for (const model of models) {
    if (model.use_cases?.includes(preferredUseCase)) preferred.push(model);
    else remaining.push(model);
  }
  return [...preferred, ...remaining];
}
