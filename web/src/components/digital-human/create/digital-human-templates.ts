/**
 * Digital human templates for the creation flow.
 *
 * Templates come from the API (ADR 0022): built-in ones in the current
 * language, then the workspace's saved ones. Each declares the skills it binds
 * and whether it needs a Connector connection; skill ids are resolved against
 * the backend's real binding candidates when the digital human is created, so a
 * template never claims a capability the workspace cannot provide.
 *
 * IMPORTANT: this module must stay free of runtime imports (type-only imports
 * are erased) so `scripts/test-digital-human-create-flow.mjs` can load it with
 * Node's type stripping.
 */

import type {
  DigitalHumanResourceCandidate,
  DigitalHumanTemplate,
} from '../../../services/types/digital-human';

export type { DigitalHumanTemplate };

export const CUSTOM_TEMPLATE_ID = 'custom';

export function isCustomTemplateId(value: string | null | undefined): boolean {
  return value === CUSTOM_TEMPLATE_ID;
}

export function findDigitalHumanTemplate(
  templates: readonly DigitalHumanTemplate[],
  value: string | null | undefined
): DigitalHumanTemplate | null {
  if (!value) return null;
  return templates.find(template => template.id === value) ?? null;
}

/** The name a new digital human starts with: a saved one's, else the title. */
export function templateStartingName(template: DigitalHumanTemplate): string {
  return template.identity_name?.trim() || template.title;
}

/** What keeps a template from creating a working digital human. */
export interface TemplateRequirementGap {
  /** Required skills the workspace does not offer. */
  missingSkillIds: string[];
  /** The template needs a Connector connection and none is selected. */
  connectorMissing: boolean;
}

/**
 * Check a template's needs against what the workspace offers and what the
 * user selected. Creation is refused while anything is missing, so a digital
 * human never starts claiming a capability it does not have.
 */
export function templateRequirementGap(
  template: DigitalHumanTemplate | null,
  candidates: readonly DigitalHumanResourceCandidate[],
  selectedSkillIds: readonly string[],
  connectorCount: number
): TemplateRequirementGap | null {
  if (!template) return null;
  const offered = new Set(
    candidates
      .filter(candidate => candidate.available !== false)
      .map(candidate => candidate.resource_id)
  );
  const selected = new Set(selectedSkillIds);
  const missingSkillIds = template.skills
    .filter(skill => skill.required && (!offered.has(skill.id) || !selected.has(skill.id)))
    .map(skill => skill.id);
  const connectorMissing = template.requires_connector && connectorCount === 0;
  return missingSkillIds.length > 0 || connectorMissing
    ? { missingSkillIds, connectorMissing }
    : null;
}
