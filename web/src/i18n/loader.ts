import { type Locale } from '@/lib/i18n';
import type { LanguageValue } from '@/lib/constants';

// Define available modules
export const AVAILABLE_MODULES = [
  'common',
  'navigation',
  'auth',
  'users',
  'dashboard',
  'settings',
  'aiProviders',
  'models',
  'ui',
  'datasets',
  'dbs',
  'agents',
  'nodes',
  'files',
  'webapp',
  'profile',
  'channels',
  'apikeys',
  'market',
  'workspace',
  'automation',
  'contentParse',
  'prompts',
] as const;

export type ModuleName = (typeof AVAILABLE_MODULES)[number];

import type { Messages as StrictMessages } from './modules';

// Interface for loaded messages
export type Messages = StrictMessages;

type ModuleMessages = Record<string, unknown>;
type LoadedModule = ModuleMessages | { default?: ModuleMessages };
type ModuleLoader = () => Promise<LoadedModule>;
type ModuleRegistry = Record<ModuleName, Record<LanguageValue, ModuleLoader>>;

function resolveLoadedModule(module: LoadedModule): ModuleMessages {
  if (typeof module === 'object' && module !== null && 'default' in module) {
    if (module.default) {
      return module.default as ModuleMessages;
    }

    return {} as ModuleMessages;
  }

  return module;
}

// Translation module loader with type safety
const moduleRegistry: ModuleRegistry = {
  common: {
    'zh-Hans': () => import('./modules/common/zh-Hans'),
    'en-US': () => import('./modules/common/en-US'),
  },
  auth: {
    'zh-Hans': () => import('./modules/auth/zh-Hans'),
    'en-US': () => import('./modules/auth/en-US'),
  },
  navigation: {
    'zh-Hans': () => import('./modules/navigation/zh-Hans'),
    'en-US': () => import('./modules/navigation/en-US'),
  },
  dashboard: {
    'zh-Hans': () => import('./modules/dashboard/zh-Hans'),
    'en-US': () => import('./modules/dashboard/en-US'),
  },
  users: {
    'zh-Hans': () => import('./modules/users/zh-Hans'),
    'en-US': () => import('./modules/users/en-US'),
  },
  settings: {
    'zh-Hans': () => import('./modules/settings/zh-Hans'),
    'en-US': () => import('./modules/settings/en-US'),
  },
  ui: {
    'zh-Hans': () => import('./modules/ui/zh-Hans'),
    'en-US': () => import('./modules/ui/en-US'),
  },
  models: {
    'zh-Hans': () => import('./modules/models/zh-Hans'),
    'en-US': () => import('./modules/models/en-US'),
  },
  aiProviders: {
    'zh-Hans': () => import('./modules/aiProviders/zh-Hans'),
    'en-US': () => import('./modules/aiProviders/en-US'),
  },
  channels: {
    'zh-Hans': () => import('./modules/channels/zh-Hans'),
    'en-US': () => import('./modules/channels/en-US'),
  },
  datasets: {
    'zh-Hans': () => import('./modules/datasets/zh-Hans'),
    'en-US': () => import('./modules/datasets/en-US'),
  },
  dbs: {
    'zh-Hans': () => import('./modules/dbs/zh-Hans'),
    'en-US': () => import('./modules/dbs/en-US'),
  },
  agents: {
    'zh-Hans': () => import('./modules/agents/zh-Hans'),
    'en-US': () => import('./modules/agents/en-US'),
  },
  nodes: {
    'zh-Hans': () => import('./modules/nodes/zh-Hans'),
    'en-US': () => import('./modules/nodes/en-US'),
  },
  files: {
    'zh-Hans': () => import('./modules/files/zh-Hans'),
    'en-US': () => import('./modules/files/en-US'),
  },
  webapp: {
    'zh-Hans': () => import('./modules/webapp/zh-Hans'),
    'en-US': () => import('./modules/webapp/en-US'),
  },
  profile: {
    'zh-Hans': () => import('./modules/profile/zh-Hans'),
    'en-US': () => import('./modules/profile/en-US'),
  },
  apikeys: {
    'zh-Hans': () => import('./modules/apikeys/zh-Hans'),
    'en-US': () => import('./modules/apikeys/en-US'),
  },
  market: {
    'zh-Hans': () => import('./modules/market/zh-Hans'),
    'en-US': () => import('./modules/market/en-US'),
  },
  workspace: {
    'zh-Hans': () => import('./modules/workspace/zh-Hans'),
    'en-US': () => import('./modules/workspace/en-US'),
  },
  automation: {
    'zh-Hans': () => import('./modules/automation/zh-Hans'),
    'en-US': () => import('./modules/automation/en-US'),
  },
  contentParse: {
    'zh-Hans': () => import('./modules/contentParse/zh-Hans'),
    'en-US': () => import('./modules/contentParse/en-US'),
  },
  prompts: {
    'zh-Hans': () => import('./modules/prompts/zh-Hans'),
    'en-US': () => import('./modules/prompts/en-US'),
  },
};

type ModuleKey = keyof ModuleRegistry;

/**
 * Load all translation modules for a given locale
 */
export async function loadAllModules(locale: LanguageValue): Promise<Messages> {
  const messages: Partial<Record<ModuleName, ModuleMessages>> = {};

  // Load all modules in parallel for better performance
  const modulePromises = (
    Object.entries(moduleRegistry) as Array<[ModuleKey, ModuleRegistry[ModuleKey]]>
  ).map(async ([key, modules]) => {
    try {
      const module = await modules[locale]();
      return [key, resolveLoadedModule(module)] as const;
    } catch (error) {
      console.warn(`Failed to load ${key} module for locale ${locale}:`, error);
      return [key, {}] as const;
    }
  });

  const loadedModules = await Promise.all(modulePromises);

  // Merge all modules into the messages object
  for (const [key, moduleData] of loadedModules) {
    messages[key] = moduleData;
  }

  return messages as Messages;
}

/**
 * Load a specific module for a given locale
 */
export async function loadModule(
  moduleKey: ModuleKey,
  locale: LanguageValue
): Promise<ModuleMessages> {
  try {
    const module = await moduleRegistry[moduleKey][locale]();
    return resolveLoadedModule(module);
  } catch (error) {
    console.warn(`Failed to load ${moduleKey} module for locale ${locale}:`, error);
    return {};
  }
}

/**
 * Load specific modules for a given locale
 */
export async function loadModules(
  modules: ModuleName[],
  locale: Locale
): Promise<Partial<Record<ModuleName, ModuleMessages>>> {
  const messages: Partial<Record<ModuleName, ModuleMessages>> = {};

  // Load specified modules in parallel
  const modulePromises = modules.map(async module => {
    const moduleMessages = await loadModule(module as ModuleKey, locale as LanguageValue);
    return { module, messages: moduleMessages };
  });

  try {
    const results = await Promise.allSettled(modulePromises);

    results.forEach((result, index) => {
      const module = modules[index];
      if (result.status === 'fulfilled') {
        messages[module] = result.value.messages;
      } else {
        console.warn(`Failed to load module ${module} for locale ${locale}:`, result.reason);
        messages[module] = {};
      }
    });

    return messages;
  } catch (error) {
    console.error(`Failed to load modules for locale ${locale}:`, error);
    return {};
  }
}
