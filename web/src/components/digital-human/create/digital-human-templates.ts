/**
 * Digital human templates for the creation flow.
 *
 * Each template declares the real skills it binds. The skill ids are workspace
 * binding candidates resolved at creation time, so a template never claims a
 * capability the workspace cannot actually provide: the create flow looks the
 * ids up, binds what exists, and reports what it bound.
 *
 * See `docs/superpowers/plans/2026-09-15-digital-human-ops-specialist-create-interaction.md`.
 *
 * IMPORTANT: this module must stay free of runtime imports (type-only imports
 * are erased) so `scripts/test-digital-human-create-flow.mjs` can load it with
 * Node's type stripping.
 */

export type DigitalHumanTemplateId = 'fileGeneration';

export interface DigitalHumanTemplateSkill {
  /** Real skill id, matched against `draft/binding-candidates?type=skill`. */
  id: string;
  /** i18n key describing what the skill gives the digital human. */
  labelKey: string;
}

export interface DigitalHumanTemplateDefaults {
  autonomyLevel: 'supervised';
  visibility: 'workspace';
  memoryEnabled: boolean;
  manualTrigger: boolean;
}

export interface DigitalHumanTemplate {
  id: DigitalHumanTemplateId;
  titleKey: string;
  shortDescriptionKey: string;
  responsibilityKey: string;
  personaKey: string;
  /** Ordered i18n leaf keys; the order defines what the UI shows and writes. */
  tagKeys: readonly string[];
  goalKeys: readonly string[];
  /**
   * Complete ordered rule set: the shared safety rules first, then the
   * template-specific rules.
   */
  ruleKeys: readonly string[];
  /** Skills bound right after creation. */
  skills: readonly DigitalHumanTemplateSkill[];
  /** Text-avatar background colour, written into the create request. */
  iconBackground: string;
  defaults: DigitalHumanTemplateDefaults;
}

/**
 * Resolves one translated string by key.
 *
 * Text resolution is injected so the template and payload modules stay pure and
 * unit-testable without a React or i18n runtime.
 */
export type RoleTextResolver = (key: string) => string;

export const CUSTOM_TEMPLATE_ID = 'custom';

const TEMPLATES_PREFIX = 'createFlow.templates';
const SHARED_RULES_PREFIX = 'createFlow.sharedRules';

function templateKey(id: DigitalHumanTemplateId, field: string): string {
  return `${TEMPLATES_PREFIX}.${id}.${field}`;
}

/**
 * Safety rules every template inherits, so a freshly created digital human never
 * starts without the credential and confirmation guardrails.
 */
export const SHARED_RULE_KEYS: readonly string[] = [
  `${SHARED_RULES_PREFIX}.credentials`,
  `${SHARED_RULES_PREFIX}.authorization`,
  `${SHARED_RULES_PREFIX}.verifyResult`,
  `${SHARED_RULES_PREFIX}.stateBlockers`,
];

const DEFAULT_TEMPLATE_DEFAULTS: DigitalHumanTemplateDefaults = {
  autonomyLevel: 'supervised',
  visibility: 'workspace',
  memoryEnabled: false,
  manualTrigger: true,
};

interface TemplateSeed {
  id: DigitalHumanTemplateId;
  iconBackground: string;
  skills: readonly DigitalHumanTemplateSkill[];
  tagNames: readonly string[];
  goalNames: readonly string[];
  ruleNames: readonly string[];
}

const TEMPLATE_SEEDS: readonly TemplateSeed[] = [
  {
    id: 'fileGeneration',
    iconBackground: '#4f46e5',
    skills: [
      {
        id: 'file-generator',
        labelKey: templateKey('fileGeneration', 'skills.fileGenerator'),
      },
    ],
    tagNames: ['fileOutput', 'structure', 'deliveryNotes'],
    goalNames: ['confirmFormat', 'produceFile', 'explainOutput', 'fallbackFormat'],
    ruleNames: ['confirmBeforeGenerating'],
  },
];

export const DIGITAL_HUMAN_TEMPLATES: readonly DigitalHumanTemplate[] = TEMPLATE_SEEDS.map(seed => ({
  id: seed.id,
  titleKey: templateKey(seed.id, 'title'),
  shortDescriptionKey: templateKey(seed.id, 'shortDescription'),
  responsibilityKey: templateKey(seed.id, 'responsibility'),
  personaKey: templateKey(seed.id, 'persona'),
  tagKeys: seed.tagNames.map(name => templateKey(seed.id, `tags.${name}`)),
  goalKeys: seed.goalNames.map(name => templateKey(seed.id, `goals.${name}`)),
  ruleKeys: [
    ...SHARED_RULE_KEYS,
    ...seed.ruleNames.map(name => templateKey(seed.id, `rules.${name}`)),
  ],
  skills: seed.skills,
  iconBackground: seed.iconBackground,
  defaults: DEFAULT_TEMPLATE_DEFAULTS,
}));

export function isCustomTemplateId(value: string | null | undefined): boolean {
  return value === CUSTOM_TEMPLATE_ID;
}

export function findDigitalHumanTemplate(
  value: string | null | undefined
): DigitalHumanTemplate | null {
  if (!value) return null;
  return DIGITAL_HUMAN_TEMPLATES.find(template => template.id === value) ?? null;
}
