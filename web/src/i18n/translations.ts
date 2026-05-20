'use client';

import { useTranslations } from 'next-intl';
import { getTranslations } from 'next-intl/server';
import { AVAILABLE_MODULES } from './loader';
import type { Messages } from './modules';

// ============================================================================
// Unified Translation Functions
// ============================================================================

/**
 * Namespace translator types for each module.
 * Provides property hints like t.common('key'), t.auth('key'), etc.
 * Preserves all next-intl methods like .rich(), .raw(), etc.
 */
export type Translators = {
  [K in keyof Messages]: ReturnType<typeof useTranslations<K>>;
};

// ============================================================================
// Dot Notation Type Utilities
// ============================================================================

/**
 * Generate all valid dot-notation keys from the Messages type.
 * This creates a union like 'common.save' | 'common' | 'agents.workflow.chat.title' | ...
 * Includes both intermediate paths and leaf nodes for scoped translation support.
 */
type DotNotationKeys<T, Prefix extends string = ''> = T extends object
  ? {
      [K in keyof T]: K extends string
        ? T[K] extends object
          ? `${Prefix}${K}` | DotNotationKeys<T[K], `${Prefix}${K}.`>
          : `${Prefix}${K}`
        : never;
    }[keyof T]
  : never;

/**
 * All valid translation keys in dot notation format.
 */
export type AllTranslationKeys = DotNotationKeys<Messages>;

/**
 * All valid paths that can be used as a prefix scope.
 * Same as AllTranslationKeys - includes both intermediate paths and leaf nodes.
 */
export type AllScopePaths = DotNotationKeys<Messages>;

/**
 * Type utility to extract subtree keys after a prefix.
 * Example: ShiftingKeys<Messages, 'workflow.editor'> returns all keys under 'workflow.editor.*'
 */
type ShiftingKeys<T, P extends string> = P extends `${infer Head}.${infer Tail}`
  ? Head extends keyof T
    ? ShiftingKeys<T[Head], Tail>
    : never
  : P extends keyof T
    ? DotNotationKeys<T[P]>
    : never;

/**
 * Scoped translation function type.
 * Provides type-safe translations for a specific namespace scope.
 */
export type ScopedTranslations<P extends string> = (
  key: ShiftingKeys<Messages, P>,
  values?: any
) => string;

/**
 * Extract sub-keys for a specific namespace.
 * Example: TranslationSuffix<'nodes'> gives all keys under 'nodes.' without the prefix.
 */
export type TranslationSuffix<NS extends keyof Messages> = DotNotationKeys<Messages[NS]>;

/** Common namespace suffixes for strict typing */
export type NodesSuffix = TranslationSuffix<'nodes'>;
export type SettingsSuffix = TranslationSuffix<'settings'>;
export type FilesSuffix = TranslationSuffix<'files'>;
export type DatasetsSuffix = TranslationSuffix<'datasets'>;
export type DbSuffix = TranslationSuffix<'dbs'>;
export type NavigationSuffix = TranslationSuffix<'navigation'>;
export type WorkspaceSuffix = TranslationSuffix<'workspace'>;
export type DashboardSuffix = TranslationSuffix<'dashboard'>;
export type AiProvidersSuffix = TranslationSuffix<'aiProviders'>;
export type AgentsSuffix = TranslationSuffix<'agents'>;

/** Full-path keys for root t() assertions */
export type NodesKey = `nodes.${NodesSuffix}`;
export type SettingsKey = `settings.${SettingsSuffix}`;
export type FilesKey = `files.${FilesSuffix}`;
export type DatasetsKey = `datasets.${DatasetsSuffix}`;
export type DbKey = `dbs.${DbSuffix}`;
export type NavigationKey = `navigation.${NavigationSuffix}`;
export type WorkspaceKey = `workspace.${WorkspaceSuffix}`;
export type DashboardKey = `dashboard.${DashboardSuffix}`;
export type AiProvidersKey = `aiProviders.${AiProvidersSuffix}`;
export type AgentsKey = `agents.${AgentsSuffix}`;

/**
 * Dot notation translator function type.
 * Provides strong typing for t('module.key') style calls.
 */
type DotNotationTranslator = (
  key: AllTranslationKeys,
  values?: Record<string, string | number | Date>
) => string;

/**
 * Unified translation type.
 *
 * This is a hybrid type that:
 * 1. Can be called as a function with dot notation: `t('common.save')`
 * 2. Has namespace properties for direct access: `t.common('save')`
 *
 * Both usages are fully type-safe with IDE auto-completion.
 */
export type UnifiedTranslations = DotNotationTranslator & Translators;

/**
 * Universal client-side translation hook.
 *
 * Supports both unified and scoped translation modes:
 *
 * **Unified mode** - Access all modules:
 * ```tsx
 * const t = useT();
 * t('common.save')       // Dot notation
 * t.common('save')       // Namespace-based
 * ```
 *
 * **Scoped mode** - Lock to a specific namespace:
 * ```tsx
 * const t = useT('workflow.editor');
 * t('title')             // workflow.editor.title
 * t('nodes.add')         // workflow.editor.nodes.add
 * ```
 */
export function useT(): UnifiedTranslations;
export function useT<P extends AllScopePaths>(scope: P): ScopedTranslations<P>;
export function useT(scope?: string): any {
  const rootT = useTranslations(scope as any);

  // When scoped, return the translator directly to preserve .rich() and other methods
  if (scope) {
    return rootT;
  }

  // For unified mode, create wrapper with namespace accessors
  const h = (key: string, values?: any) => (rootT as any)(key, values);
  const t = h as any;

  AVAILABLE_MODULES.forEach(module => {
    // @ts-ignore - dynamic assignment
    t[module] = useTranslations(module);
  });

  return t;
}

/**
 * Universal server-side translation function.
 *
 * Supports both unified and scoped translation modes:
 *
 * **Unified mode** - Access all modules:
 * ```tsx
 * const t = await getT();
 * t('common.save')       // Dot notation
 * t.common('save')       // Namespace-based
 * ```
 *
 * **Scoped mode** - Lock to a specific namespace:
 * ```tsx
 * const t = await getT('workflow.editor');
 * t('title')             // workflow.editor.title
 * t('nodes.add')         // workflow.editor.nodes.add
 * ```
 */
export async function getT(): Promise<UnifiedTranslations>;
export async function getT<P extends AllScopePaths>(scope: P): Promise<ScopedTranslations<P>>;
export async function getT(scope?: string): Promise<any> {
  const rootT = await getTranslations(scope as any);

  // When scoped, return the translator directly to preserve .rich() and other methods
  if (scope) {
    return rootT;
  }

  // For unified mode, create wrapper with namespace accessors
  const h = (key: string, values?: any) => (rootT as any)(key, values);
  const t = h as any;

  await Promise.all(
    AVAILABLE_MODULES.map(async module => {
      // @ts-ignore - dynamic assignment
      t[module] = await getTranslations(module);
    })
  );

  return t;
}
