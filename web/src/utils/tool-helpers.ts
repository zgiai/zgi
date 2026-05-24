import type { Locale } from '@/lib/i18n';
import type { LocalizedString } from '@/services/types/system-settings';
import type {
  ToolParameter,
  ToolParameterType,
  ToolFormField,
  FormFieldType,
  LocalizedStringPartial,
  ToolParameterBinding,
  BuiltinToolProvider,
  BuiltinToolItem,
  ToolDescription,
} from '@/services/types/tool';

/**
 * Pick best localized label from LocalizedString(Provider) or LocalizedStringPartial.
 * Fallback order: `locale` -> `en_US` -> `zh_Hans` -> first available -> provided fallback.
 */
export function pickLocale(
  label: LocalizedStringPartial | LocalizedString | undefined,
  locale: Locale,
  fallback = ''
): string {
  if (!label) return fallback;
  const key = locale.replace('-', '_');
  const dict = label as Record<string, string | undefined>;
  const direct = dict[key];
  if (direct) return direct;
  if ((label as { en_US?: string }).en_US) {
    return (label as { en_US?: string }).en_US as string;
  }
  if ((label as { zh_Hans?: string }).zh_Hans) {
    return (label as { zh_Hans?: string }).zh_Hans as string;
  }
  const first = Object.values(label).find(v => typeof v === 'string' && v.length > 0);
  return first ?? fallback;
}

/**
 * Pick localized description from ToolDescription or LocalizedStringPartial.
 * Handles the new { human: I18nText, llm: string } structure.
 */
export function pickToolDescription(
  description: ToolDescription | LocalizedStringPartial | undefined,
  locale: Locale,
  fallback = ''
): string {
  if (!description) return fallback;
  // Check if it's a ToolDescription with human/llm structure
  if ('human' in description || 'llm' in description) {
    const toolDesc = description as ToolDescription;
    // Prefer human description for display
    if (toolDesc.human) {
      return pickLocale(toolDesc.human, locale, fallback);
    }
    // Fallback to llm description
    if (typeof toolDesc.llm === 'string') {
      return toolDesc.llm;
    }
    return fallback;
  }
  // It's a plain LocalizedStringPartial
  return pickLocale(description as LocalizedStringPartial, locale, fallback);
}

/** Map backend parameter type to UI form field type */
export function inferFormFieldType(type: ToolParameterType): FormFieldType {
  switch (type) {
    case 'string':
      return 'text';
    case 'number':
      return 'number';
    case 'boolean':
      return 'checkbox';
    case 'select':
      return 'select';
    case 'secret-input':
      return 'secret';
    case 'file':
      return 'file';
    default:
      // Exhaustiveness check
      return 'text';
  }
}

/** Convert backend parameters to normalized form fields for UI rendering */
export function mapParametersToFormFields(
  parameters: ToolParameter[],
  locale: Locale
): ToolFormField[] {
  return parameters.map<ToolFormField>(p => {
    const type = inferFormFieldType(p.type);
    const options = p.options?.map(opt => ({
      value: opt.value,
      label: pickLocale(opt.label, locale, String(opt.value)),
    }));
    // Get label from localized label field or fallback to name
    const label = p.label ? pickLocale(p.label, locale, p.name) : p.name;
    // Get description from human_description or legacy description
    const description = p.human_description
      ? pickLocale(p.human_description, locale, p.description ?? undefined)
      : (p.description ?? undefined);
    return {
      name: p.name,
      label,
      type,
      required: !!p.required,
      description,
      default: p.default,
      options,
    };
  });
}

/** Build a default binding for a given field */
export function createDefaultBinding(field: ToolFormField): ToolParameterBinding {
  if (field.default !== undefined) {
    if (field.type === 'text') {
      return { type: 'mixed', value: field.default };
    }
    return { type: 'constant', value: field.default };
  }
  if (field.type === 'text') {
    return { type: 'mixed', value: '' };
  }
  // No default provided; prefer variable mode to encourage workflow linking
  return { type: 'variable', value: undefined };
}

/** Coerce a raw value to the correct type based on field definition */
export function coerceValue(
  field: ToolFormField,
  raw: unknown
): string | number | boolean | undefined {
  if (raw === undefined || raw === null) return undefined;
  switch (field.type) {
    case 'checkbox':
      return Boolean(raw);
    case 'number':
      if (typeof raw === 'number') return raw;
      if (typeof raw === 'string') {
        const n = Number(raw);
        return Number.isNaN(n) ? undefined : n;
      }
      return undefined;
    case 'select':
      return typeof raw === 'string' ? raw : String(raw);
    case 'secret':
    case 'text':
    case 'file':
      return typeof raw === 'string' ? raw : String(raw);
    default:
      return undefined;
  }
}

/** Extract defaults from parameter list to initialize form state */
export function collectParameterDefaults(
  parameters: ToolParameter[]
): Record<string, string | number | boolean | undefined> {
  const result: Record<string, string | number | boolean | undefined> = {};
  for (const p of parameters) {
    if (p.default !== undefined) {
      result[p.name] = p.default;
    }
  }
  return result;
}

/** Find a tool by provider id and tool name */
export function getToolByIdentity(
  providers: BuiltinToolProvider[],
  providerId: string,
  toolName: string
): BuiltinToolItem | undefined {
  const p = providers.find(x => x.id === providerId || x.name === providerId);
  if (!p) return undefined;
  return p.tools.find(t => t.name === toolName);
}

/**
 * Resolve a record of bindings into a plain payload for execution
 * - variableResolver returns a runtime value for a variable id
 */
export function resolveBindingsToPayload(
  bindings: Record<string, ToolParameterBinding | undefined>,
  variableResolver: (variable: string[] | undefined) => unknown
): Record<string, unknown> {
  const payload: Record<string, unknown> = {};
  for (const [name, binding] of Object.entries(bindings)) {
    if (!binding) continue;
    if (binding.type === 'constant') {
      payload[name] = binding.value;
    } else {
      payload[name] = variableResolver(binding.value as string[] | undefined);
    }
  }
  return payload;
}

/** Create initial bindings map from form fields */
export function createInitialBindings(
  fields: ToolFormField[]
): Record<string, ToolParameterBinding> {
  const map: Record<string, ToolParameterBinding> = {};
  for (const f of fields) {
    map[f.name] = createDefaultBinding(f);
  }
  return map;
}
