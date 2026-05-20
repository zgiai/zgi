import type { ParameterRuleItem } from '@/services/types/model';

const MODEL_CONFIG_PARAMETER_TEMPLATE_KEY_ALLOWLIST = new Set([
  'temperature',
  'top_p',
  'presence_penalty',
  'frequency_penalty',
  'logit_bias',
  'seed',
  'stop',
  'max_tokens',
]);

interface ResolveModelConfigParameterCopyOptions {
  parameter: Pick<ParameterRuleItem, 'name' | 'template_key'>;
  translate: (key: string) => string;
}

interface ModelConfigParameterCopy {
  help: string;
  label: string;
}

function safeTranslate(translate: (key: string) => string, key: string, fallback: string): string {
  try {
    const translated = translate(key);
    if (!translated || translated === key) {
      return fallback;
    }
    return translated;
  } catch {
    return fallback;
  }
}

/**
 * @util Resolve model config parameter copy with strict whitelist and fallback behavior.
 * Unknown template keys never call i18n and always fall back to the raw parameter name.
 */
export function resolveModelConfigParameterCopy({
  parameter,
  translate,
}: ResolveModelConfigParameterCopyOptions): ModelConfigParameterCopy {
  const fallbackLabel = parameter.name;
  const templateKey = parameter.template_key?.trim();

  if (!templateKey || !MODEL_CONFIG_PARAMETER_TEMPLATE_KEY_ALLOWLIST.has(templateKey)) {
    return {
      help: '',
      label: fallbackLabel,
    };
  }

  return {
    help: safeTranslate(translate, `models.configParameters.templates.${templateKey}.help`, ''),
    label: safeTranslate(
      translate,
      `models.configParameters.templates.${templateKey}.label`,
      fallbackLabel
    ),
  };
}

export function isModelConfigParameterTemplateKeySupported(templateKey?: string | null): boolean {
  if (!templateKey) {
    return false;
  }

  return MODEL_CONFIG_PARAMETER_TEMPLATE_KEY_ALLOWLIST.has(templateKey.trim());
}
