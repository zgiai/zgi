import type { PromptType } from '@/services/types/prompt';

export function promptLocaleLabelKey(locale?: string) {
  switch (locale) {
    case 'zh-Hans':
      return 'localeOptions.zhHans';
    case 'en-US':
      return 'localeOptions.enUS';
    case 'ja-JP':
      return 'localeOptions.jaJP';
    default:
      return 'localeOptions.unknown';
  }
}

export function promptTypeLabelKey(type?: PromptType | string) {
  return type === 'chat' ? 'promptTypes.chat' : 'promptTypes.text';
}
