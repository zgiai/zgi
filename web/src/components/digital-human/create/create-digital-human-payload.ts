/**
 * Pure mapping from the creation form state to the create request, plus the
 * skill bindings a template asks for.
 *
 * Keeping these pure is what lets `scripts/test-digital-human-create-flow.mjs`
 * prove, without rendering anything, exactly what a template selection creates
 * and which real skills it binds.
 *
 * IMPORTANT: only type-only imports may appear here, so the module stays
 * loadable by Node's type stripping in tests.
 */

import type { IconValue } from '../../common/icon-input/types';
import type {
  CreateDigitalHumanRequest,
  DigitalHumanResourceBindingInput,
  DigitalHumanResourceCandidate,
} from '../../../services/types/digital-human';
import type { DigitalHumanTemplate } from './digital-human-templates';

/** Fallback text-avatar glyph when neither the avatar nor the name yields one. */
const FALLBACK_ICON_TEXT = 'AI';

/** Fallback text-avatar colour, used when neither the avatar nor the template provides one. */
const FALLBACK_ICON_BACKGROUND = '#0847f7';

export interface DigitalHumanCreateFormState {
  name: string;
  responsibility: string;
  icon: IconValue;
  model: string;
}

export interface CreateDigitalHumanPayloadInput extends DigitalHumanCreateFormState {
  /** `null` means the custom (blank) entry: the responsibility is the persona. */
  template: DigitalHumanTemplate | null;
  /** The safety rules every digital human starts with, from the catalog. */
  sharedRules: readonly string[];
}

/**
 * Compose the persona written into the new digital human (ADR 0022).
 *
 * It is the template's persona. A responsibility the user changed is added as
 * a plain paragraph, so the persona stays in one language and holds no
 * instructions meant for anyone but the digital human.
 */
export function buildPersona(template: DigitalHumanTemplate, responsibility: string): string {
  const persona = template.persona.trim();
  const duty = responsibility.trim();
  if (!duty || duty === template.responsibility.trim()) return persona;
  return [persona, duty].filter(Boolean).join('\n\n');
}

/** Map the avatar selection onto the wire format the create API expects. */
export function serializeDigitalHumanIcon(
  icon: IconValue,
  name: string,
  fallbackBackground: string
): Pick<CreateDigitalHumanRequest, 'icon_type' | 'icon'> {
  if (icon.type === 'image') {
    return { icon_type: 'image', icon: icon.imageId || icon.iconUrl || '' };
  }

  const text = (icon.icon || name.trim().slice(0, 2).toUpperCase() || FALLBACK_ICON_TEXT).trim();
  return {
    icon_type: 'text',
    icon: JSON.stringify({
      icon: text,
      icon_background: icon.iconBackground || fallbackBackground || FALLBACK_ICON_BACKGROUND,
    }),
  };
}

/**
 * Build the create request for a template or the custom (blank) entry.
 *
 * Skills never enter this request: they are bound to the created draft through
 * the bindings API, because the create API has no binding field and stuffing a
 * capability claim into `description`/`persona` would fake it.
 */
export function createDigitalHumanPayload(
  input: CreateDigitalHumanPayloadInput
): CreateDigitalHumanRequest {
  const { template } = input;
  const name = input.name.trim();
  const responsibility = input.responsibility.trim();
  return {
    name,
    // A built-in template's required items gate publishing (ADR 0023).
    ...(template?.source === 'builtin' ? { template_id: template.id } : {}),
    description: responsibility,
    ...serializeDigitalHumanIcon(
      input.icon,
      name,
      template?.icon_background ?? FALLBACK_ICON_BACKGROUND
    ),
    identity_name: name,
    persona: template ? buildPersona(template, responsibility) : responsibility,
    goals: template ? [...template.goals] : [],
    // The blank entry starts with the shared safety rules too.
    rules: template ? [...template.rules] : [...input.sharedRules],
    default_model: input.model,
    autonomy_level: 'supervised',
    visibility: 'workspace',
    // Only what the runtime reads: memory is always on (ADR 0019), and the
    // template's memory instructions guide it.
    memory_policy: template?.memory_instructions.trim()
      ? { instructions: template.memory_instructions.trim() }
      : {},
    trigger_policy: { manual: true },
  };
}

/**
 * Resolve the skills a template asks for against the candidates the workspace
 * actually offers.
 *
 * A template must never claim a capability the workspace cannot provide, so
 * unknown or unavailable skills are skipped instead of being invented. The
 * caller reports which of them were bound.
 */
export function selectTemplateBindings(
  template: DigitalHumanTemplate | null,
  candidates: readonly DigitalHumanResourceCandidate[]
): DigitalHumanResourceBindingInput[] {
  if (!template) return [];
  return selectCandidateBindings(
    template.skills.map(skill => skill.id),
    candidates
  );
}

/**
 * Resource ids of the skills a template offers as the initial selection.
 *
 * Creation lets the user adjust the selection, so the template only seeds it:
 * ids the workspace does not offer are dropped here exactly like they are
 * dropped when binding.
 */
export function templateDefaultSkillIds(
  template: DigitalHumanTemplate | null,
  candidates: readonly DigitalHumanResourceCandidate[]
): string[] {
  if (!template) return [];
  const offered = new Map(
    candidates
      .filter(candidate => candidate.available !== false)
      .map(candidate => [candidate.resource_id, candidate])
  );
  return template.skills.map(skill => skill.id).filter(id => offered.has(id));
}

/**
 * Turn an explicit skill selection into binding inputs.
 *
 * Only candidates the workspace really offers can be bound, and the selection
 * order is normalised to the candidate order so the request is stable.
 */
export function selectCandidateBindings(
  selectedIds: readonly string[],
  candidates: readonly DigitalHumanResourceCandidate[]
): DigitalHumanResourceBindingInput[] {
  const selected = new Set(selectedIds);
  const items: DigitalHumanResourceBindingInput[] = [];

  for (const candidate of candidates) {
    if (!selected.has(candidate.resource_id) || candidate.available === false) continue;
    items.push({
      type: 'skill',
      resource_id: candidate.resource_id,
      parent_resource_id: candidate.parent_resource_id,
      display_name: candidate.display_name,
      access_mode: candidate.access_modes?.[0] ?? 'execute',
      resource_revision: candidate.resource_revision,
      metadata: candidate.metadata ?? {},
    });
  }

  return items;
}

/** Display names of the bindings that were actually written. */
export function bindingDisplayNames(
  items: ReadonlyArray<Pick<DigitalHumanResourceBindingInput, 'display_name' | 'resource_id'>>
): string[] {
  return items.map(item => item.display_name || item.resource_id);
}
