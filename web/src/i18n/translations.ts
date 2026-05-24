'use client';

/* eslint-disable no-redeclare, @typescript-eslint/no-explicit-any */

import { useTranslations } from 'next-intl';
import { getTranslations } from 'next-intl/server';
import type { ReactNode } from 'react';
import { AVAILABLE_MODULES, type ModuleName } from './loader';
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
  values?: Record<string, unknown>
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

interface RuntimeTranslator {
  (key: string, values?: Record<string, unknown>): string;
  rich?: (key: string, values?: Record<string, unknown>) => ReactNode;
  markup?: (key: string, values?: Record<string, unknown>) => string;
  raw?: (key: string) => unknown;
  has?: (key: string) => boolean;
}

type UnifiedRuntimeTranslator = RuntimeTranslator & Partial<Record<ModuleName, RuntimeTranslator>>;

const warnedMissingKeys = new Set<string>();

function shouldReportMissingTranslations(): boolean {
  return process.env.NODE_ENV !== 'production';
}

function reportMissingTranslation(key: string): void {
  if (!shouldReportMissingTranslations() || warnedMissingKeys.has(key)) {
    return;
  }

  warnedMissingKeys.add(key);
  console.error(`[i18n] Missing message for "${key}". Check route module coverage.`);
}

function hasTranslation(translator: RuntimeTranslator, key: string): boolean {
  return typeof translator.has !== 'function' || translator.has(key);
}

function callWithMissingKeyReport(
  translator: RuntimeTranslator,
  key: string,
  values?: Record<string, unknown>
): string {
  if (!hasTranslation(translator, key)) {
    reportMissingTranslation(key);
  }

  return translator(key, values);
}

function createScopedTranslatorWithMissingKeyReport(
  translator: RuntimeTranslator,
  scope: string
): RuntimeTranslator {
  const scoped = ((key: string, values?: Record<string, unknown>) => {
    if (!hasTranslation(translator, key)) {
      reportMissingTranslation(`${scope}.${key}`);
    }

    return translator(key, values);
  }) as RuntimeTranslator;

  if (typeof translator.rich === 'function') {
    scoped.rich = (key, values) => {
      if (!hasTranslation(translator, key)) {
        reportMissingTranslation(`${scope}.${key}`);
      }
      return translator.rich?.(key, values) ?? '';
    };
  }
  if (typeof translator.markup === 'function') {
    scoped.markup = (key, values) => {
      if (!hasTranslation(translator, key)) {
        reportMissingTranslation(`${scope}.${key}`);
      }
      return translator.markup?.(key, values) ?? '';
    };
  }
  if (typeof translator.raw === 'function') {
    scoped.raw = key => {
      if (!hasTranslation(translator, key)) {
        reportMissingTranslation(`${scope}.${key}`);
      }
      return translator.raw?.(key);
    };
  }
  if (typeof translator.has === 'function') {
    scoped.has = translator.has.bind(translator);
  }

  return scoped;
}

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

function bindRootMethods(rootT: RuntimeTranslator, translator: RuntimeTranslator): void {
  if (typeof rootT.rich === 'function') {
    translator.rich = rootT.rich.bind(rootT);
  }
  if (typeof rootT.markup === 'function') {
    translator.markup = rootT.markup.bind(rootT);
  }
  if (typeof rootT.raw === 'function') {
    translator.raw = rootT.raw.bind(rootT);
  }
  if (typeof rootT.has === 'function') {
    translator.has = rootT.has.bind(rootT);
  }
}

function createNamespaceTranslator(
  rootT: RuntimeTranslator,
  module: ModuleName
): RuntimeTranslator {
  const scoped = ((key: string, values?: Record<string, unknown>) =>
    callWithMissingKeyReport(rootT, `${module}.${key}`, values)) as RuntimeTranslator;

  if (typeof rootT.rich === 'function') {
    scoped.rich = (key, values) => {
      const fullKey = `${module}.${key}`;
      if (!hasTranslation(rootT, fullKey)) {
        reportMissingTranslation(fullKey);
      }
      return rootT.rich?.(fullKey, values) ?? '';
    };
  }
  if (typeof rootT.markup === 'function') {
    scoped.markup = (key, values) => {
      const fullKey = `${module}.${key}`;
      if (!hasTranslation(rootT, fullKey)) {
        reportMissingTranslation(fullKey);
      }
      return rootT.markup?.(fullKey, values) ?? '';
    };
  }
  if (typeof rootT.raw === 'function') {
    scoped.raw = key => rootT.raw?.(`${module}.${key}`);
  }
  if (typeof rootT.has === 'function') {
    scoped.has = key => rootT.has?.(`${module}.${key}`) ?? false;
  }

  return scoped;
}

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
    return createScopedTranslatorWithMissingKeyReport(rootT as RuntimeTranslator, scope);
  }

  // For unified mode, create wrapper with namespace accessors
  const h = ((key: string, values?: Record<string, unknown>) =>
    callWithMissingKeyReport(rootT as RuntimeTranslator, key, values)) as RuntimeTranslator;
  bindRootMethods(rootT as RuntimeTranslator, h);
  const t = h as UnifiedRuntimeTranslator;

  AVAILABLE_MODULES.forEach(module => {
    t[module] = createNamespaceTranslator(rootT as RuntimeTranslator, module);
  });

  return t as UnifiedTranslations;
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
    return createScopedTranslatorWithMissingKeyReport(rootT as RuntimeTranslator, scope);
  }

  // For unified mode, create wrapper with namespace accessors
  const h = ((key: string, values?: Record<string, unknown>) =>
    callWithMissingKeyReport(rootT as RuntimeTranslator, key, values)) as RuntimeTranslator;
  bindRootMethods(rootT as RuntimeTranslator, h);
  const t = h as UnifiedRuntimeTranslator;

  AVAILABLE_MODULES.forEach(module => {
    t[module] = createNamespaceTranslator(rootT as RuntimeTranslator, module);
  });

  return t as UnifiedTranslations;
}
