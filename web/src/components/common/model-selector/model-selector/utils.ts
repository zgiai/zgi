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
