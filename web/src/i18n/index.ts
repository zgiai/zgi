// i18n barrel export
export { locales, defaultLocale, localeNames, localeMapping, type Locale } from './config';

export { loadAllModules } from './loader';
export type { Messages } from './modules';

// Translation hooks/functions
export {
  useT,
  getT,
  type UnifiedTranslations,
  type AllTranslationKeys,
  type NodesSuffix,
  type SettingsSuffix,
  type FilesSuffix,
  type DatasetsSuffix,
  type DbSuffix,
  type NavigationSuffix,
  type WorkspaceSuffix,
  type DashboardSuffix,
  type AiProvidersSuffix,
  type AgentsSuffix,
  type NodesKey,
  type SettingsKey,
  type FilesKey,
  type DatasetsKey,
  type DbKey,
  type NavigationKey,
  type WorkspaceKey,
  type DashboardKey,
  type AiProvidersKey,
  type AgentsKey,
} from './translations';
