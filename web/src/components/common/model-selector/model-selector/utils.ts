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

export function modelDisplayVendor(model: ModelItem): string {
  if (model.is_custom) return model.provider;
  return model.vendor?.trim().toLowerCase() || '';
}

export function aggregatePlatformModels(models: ModelItem[]): ModelItem[] {
  const result: ModelItem[] = [];
  const platformIndexes = new Map<string, number>();

  for (const model of models) {
    if (model.is_custom) {
      result.push(model);
      continue;
    }

    const vendor = modelDisplayVendor(model);
    if (!vendor) continue;
    const key = `${vendor}\u0000${model.model}`;
    const existingIndex = platformIndexes.get(key);
    if (existingIndex === undefined) {
      platformIndexes.set(key, result.length);
      result.push(normalizePlatformVendor(model, vendor));
      continue;
    }

    result[existingIndex] = mergePlatformModel(result[existingIndex], model);
  }

  return result;
}

function normalizePlatformVendor(model: ModelItem, vendor: string): ModelItem {
  const sourceVendor = model.vendor?.trim().toLowerCase() || model.provider.trim().toLowerCase();
  if (sourceVendor === vendor) return { ...model, vendor };
  return {
    ...model,
    vendor,
    vendor_name: undefined,
    vendor_cn_name: undefined,
    vendor_en_name: undefined,
  };
}

function mergePlatformModel(current: ModelItem, incoming: ModelItem): ModelItem {
  const currentVendor = modelDisplayVendor(current);
  const incomingVendor = modelDisplayVendor(incoming);
  const primary = preferPlatformModel(current, incoming, currentVendor, incomingVendor) ? current : incoming;
  const secondary = primary === current ? incoming : current;
  const resolvedVendor = primary === current ? currentVendor : incomingVendor;
  const secondaryDeclaredVendor =
    secondary.vendor?.trim().toLowerCase() || secondary.provider.trim().toLowerCase();
  const secondaryHasSameVendor = secondaryDeclaredVendor === resolvedVendor;
  return {
    ...secondary,
    ...primary,
    vendor: resolvedVendor,
    vendor_name: primary.vendor_name || (secondaryHasSameVendor ? secondary.vendor_name : undefined),
    vendor_cn_name: primary.vendor_cn_name || (secondaryHasSameVendor ? secondary.vendor_cn_name : undefined),
    vendor_en_name: primary.vendor_en_name || (secondaryHasSameVendor ? secondary.vendor_en_name : undefined),
    is_flagship: Boolean(current.is_flagship || incoming.is_flagship),
    is_recommended: Boolean(current.is_recommended || incoming.is_recommended),
    is_featured: Boolean(current.is_featured || incoming.is_featured),
    is_new: Boolean(current.is_new || incoming.is_new),
    callable: Boolean(current.callable || incoming.callable),
    is_available: Boolean(current.is_available || incoming.is_available),
    is_configured: Boolean(current.is_configured || incoming.is_configured),
    use_cases: union(current.use_cases, incoming.use_cases),
    input_modalities: union(current.input_modalities, incoming.input_modalities),
    output_modalities: union(current.output_modalities, incoming.output_modalities),
    endpoints: mergeBooleanRecord(current.endpoints, incoming.endpoints),
    features: mergeBooleanRecord(current.features, incoming.features),
    tools: mergeBooleanRecord(current.tools, incoming.tools),
  };
}

function preferPlatformModel(
  current: ModelItem,
  incoming: ModelItem,
  currentVendor: string,
  incomingVendor: string
): boolean {
  const rank = (model: ModelItem, vendor: string) =>
    modelOwnerAffinity(model.model, vendor) +
    (model.provider.toLowerCase() === vendor ? 8 : 0) +
    (model.callable ? 4 : 0) +
    (model.is_recommended ? 2 : 0) +
    (model.is_available ? 1 : 0);
  const currentRank = rank(current, currentVendor);
  const incomingRank = rank(incoming, incomingVendor);
  if (currentRank !== incomingRank) return currentRank > incomingRank;
  return `${current.provider}\u0000${current.id}` <= `${incoming.provider}\u0000${incoming.id}`;
}

function modelOwnerAffinity(model: string, vendor: string): number {
  const normalizedModel = model.trim().toLowerCase();
  const normalizedVendor = vendor.trim().toLowerCase();
  if (!normalizedModel || !normalizedVendor) return 0;
  return normalizedModel === normalizedVendor ||
    normalizedModel.startsWith(`${normalizedVendor}-`) ||
    normalizedModel.startsWith(`${normalizedVendor}/`)
    ? 16
    : 0;
}

function union<T>(left: T[] | null | undefined, right: T[] | null | undefined): T[] {
  return Array.from(new Set([...(left ?? []), ...(right ?? [])]));
}

function mergeBooleanRecord<T extends object>(left: T | undefined, right: T | undefined): T {
  const merged = { ...(left ?? {}), ...(right ?? {}) } as Record<string, unknown>;
  for (const key of new Set([...Object.keys(left ?? {}), ...Object.keys(right ?? {})])) {
    const leftValue = (left as Record<string, unknown> | undefined)?.[key];
    const rightValue = (right as Record<string, unknown> | undefined)?.[key];
    if (typeof leftValue === 'boolean' || typeof rightValue === 'boolean') {
      merged[key] = Boolean(leftValue || rightValue);
    }
  }
  return merged as T;
}
