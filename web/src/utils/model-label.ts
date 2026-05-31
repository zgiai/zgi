import type { Locale } from '@/lib/i18n';
import type { ModelItem } from '@/services/types/model';

interface LocalizedModelLabel {
  model_name?: string;
  display_name?: string;
  name?: string;
}

type ModelLabelSource = Pick<ModelItem, 'model' | 'model_name'> & {
  display_name?: string;
  name?: string;
  i18n?: Record<string, LocalizedModelLabel>;
  zh_Hans?: string;
  en_US?: string;
};

const CJK_RE = /[\u3400-\u9fff]/;

function firstNonEmpty(values: Array<string | undefined>): string | undefined {
  return values.find(value => typeof value === 'string' && value.trim().length > 0)?.trim();
}

function getLocaleKeys(locale?: Locale | string): string[] {
  return locale?.toLowerCase().startsWith('zh')
    ? ['zh_Hans', 'zh-Hans', 'zh_CN', 'zh-CN', 'zh']
    : ['en_US', 'en-US', 'en'];
}

export function getModelDisplayName(model: ModelLabelSource, locale?: Locale | string): string {
  const localized = firstNonEmpty(
    getLocaleKeys(locale).flatMap(key => [
      model.i18n?.[key]?.model_name,
      model.i18n?.[key]?.display_name,
      model.i18n?.[key]?.name,
      key === 'zh_Hans' ? model.zh_Hans : undefined,
      key === 'en_US' ? model.en_US : undefined,
    ])
  );

  if (localized) return localized;

  const fallback = firstNonEmpty([model.model_name, model.display_name, model.name, model.model]);
  if (!locale?.toLowerCase().startsWith('zh') && fallback && CJK_RE.test(fallback)) {
    return model.model || fallback;
  }

  return fallback || model.model;
}
