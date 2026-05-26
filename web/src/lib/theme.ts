// Theme management system

import { ENABLE_THEME_SWITCH, DEFAULT_THEME_NAME } from '@/lib/config';

export type Theme =
  | 'light'
  | 'dark'
  | 'tech-blue'
  | 'graphite-cyan'
  | 'emerald'
  | 'violet'
  | 'warm-orange';

export interface ThemeConfig {
  name: string;
  displayName: string;
  description?: string;
  preview?: {
    primary: string;
    secondary: string;
    background: string;
  };
}

// Available themes configuration
export const THEMES: Record<Theme, ThemeConfig> = {
  light: {
    name: 'light',
    displayName: 'Default Blue',
    description: 'Chainlink-inspired blue, cool canvas, and restrained action states',
    preview: {
      primary: '#0847F7',
      secondary: '#DFE7FB',
      background: '#ffffff',
    },
  },
  dark: {
    name: 'dark',
    displayName: 'Dark',
    description: 'Easy on the eyes',
    preview: {
      primary: '#ffffff',
      secondary: '#404040',
      background: '#1a1a1a',
    },
  },
  'graphite-cyan': {
    name: 'graphite-cyan',
    displayName: 'Graphite Cyan',
    description: 'Gray-white console with restrained cyan actions',
    preview: {
      primary: '#0E7490',
      secondary: '#ECFEFF',
      background: '#ffffff',
    },
  },
  emerald: {
    name: 'emerald',
    displayName: 'Emerald',
    description: 'Fresh green accent for operational workflows',
    preview: {
      primary: '#059669',
      secondary: '#ECFDF5',
      background: '#ffffff',
    },
  },
  violet: {
    name: 'violet',
    displayName: 'Violet',
    description: 'Vibrant purple primary on light background',
    preview: {
      primary: '#6E56CF',
      secondary: '#F1EEFE',
      background: '#ffffff',
    },
  },
  'warm-orange': {
    name: 'warm-orange',
    displayName: 'Warm Orange',
    description: 'Warm orange primary on light background',
    preview: {
      primary: '#C26A00',
      secondary: '#FFF7ED',
      background: '#ffffff',
    },
  },
  'tech-blue': {
    name: 'tech-blue',
    displayName: 'Tech Blue',
    description: 'Deep blue primary on white background',
    preview: {
      primary: '#2450DE',
      secondary: '#eaf0ff',
      background: '#ffffff',
    },
  },
};

const TEMPORARILY_DISABLED_THEMES: readonly Theme[] = ['dark'];
const LEGACY_THEME_ALIASES: Record<string, Theme> = {
  'ai-young': 'violet',
  'whale-orange': 'warm-orange',
};

export function normalizeTheme(theme: string | undefined): Theme {
  const themeName = theme?.trim();
  const aliasedTheme = themeName ? LEGACY_THEME_ALIASES[themeName] || themeName : 'light';
  const resolvedTheme = aliasedTheme in THEMES ? (aliasedTheme as Theme) : 'light';
  return TEMPORARILY_DISABLED_THEMES.includes(resolvedTheme) ? 'light' : resolvedTheme;
}

// Resolve default theme from env config, fallback to 'light' if invalid
export const DEFAULT_THEME: Theme = normalizeTheme(DEFAULT_THEME_NAME);

// Theme storage key
const THEME_STORAGE_KEY = 'theme';

/**
 * Get the current theme from localStorage or default
 * When theme switching is disabled, always return DEFAULT_THEME
 */
export function getCurrentTheme(): Theme {
  if (typeof window === 'undefined') {
    return DEFAULT_THEME;
  }

  // When theme switching is disabled, always use default theme
  if (!ENABLE_THEME_SWITCH) {
    return DEFAULT_THEME;
  }

  try {
    const stored = localStorage.getItem(THEME_STORAGE_KEY);
    if (stored) {
      const resolvedTheme = normalizeTheme(stored);
      if (stored !== resolvedTheme) {
        localStorage.setItem(THEME_STORAGE_KEY, resolvedTheme);
      }
      return resolvedTheme;
    }
  } catch (error) {
    console.warn('Failed to read theme from localStorage:', error);
  }

  // No system preference auto-detection, use default theme for consistency
  return DEFAULT_THEME;
}

/**
 * Save theme to localStorage
 */
export function saveTheme(theme: Theme): void {
  if (typeof window === 'undefined') {
    return;
  }

  try {
    localStorage.setItem(THEME_STORAGE_KEY, normalizeTheme(theme));
  } catch (error) {
    console.warn('Failed to save theme to localStorage:', error);
  }
}

/**
 * Apply theme to document
 */
export function applyTheme(theme: Theme): void {
  if (typeof window === 'undefined') {
    return;
  }

  const resolvedTheme = normalizeTheme(theme);
  const { documentElement } = document;
  const body = document.body;

  // Remove all theme classes
  Object.keys(THEMES).forEach(themeKey => {
    documentElement.classList.remove(themeKey);
    body.classList.remove(themeKey);
  });

  // Remove data-theme attribute
  documentElement.removeAttribute('data-theme');

  // Apply new theme
  if (resolvedTheme !== 'light') {
    documentElement.setAttribute('data-theme', resolvedTheme);
  }

  // Save to localStorage
  saveTheme(resolvedTheme);

  // Dispatch theme change event
  window.dispatchEvent(new CustomEvent('theme-change', { detail: { theme: resolvedTheme } }));
}

/**
 * Dark mode is temporarily disabled, so quick toggles fall back to light.
 */
export function toggleTheme(): Theme {
  const newTheme: Theme = 'light';
  applyTheme(newTheme);
  return newTheme;
}

/**
 * Get theme configuration
 */
export function getThemeConfig(theme: Theme): ThemeConfig {
  return THEMES[normalizeTheme(theme)];
}

/**
 * Get all available themes
 */
export function getAllThemes(): ThemeConfig[] {
  return Object.entries(THEMES)
    .filter(([themeName]) => normalizeTheme(themeName as Theme) === themeName)
    .map(([, config]) => config);
}

/**
 * Check if theme is dark
 */
export function isDarkTheme(theme: Theme): boolean {
  return normalizeTheme(theme) === 'dark';
}

/**
 * Initialize theme system
 */
export function initializeTheme(): Theme {
  const theme = getCurrentTheme();
  applyTheme(theme);
  return theme;
}

/**
 * Listen to system theme changes
 * Disabled when theme switching is not allowed
 */
export function setupSystemThemeListener(): () => void {
  // Disable system theme listener when theme switching is disabled
  if (typeof window === 'undefined' || !ENABLE_THEME_SWITCH) {
    return () => {};
  }

  // System theme auto-detection is disabled for consistency
  // Users must manually select their preferred theme
  return () => {};
}

/**
 * Theme class utilities for dynamic styling
 */
export const themeClasses = {
  // Primary styles
  primary: 'theme-primary',
  primaryHover: 'theme-primary-hover',

  // Secondary styles
  secondary: 'theme-secondary',
  secondaryHover: 'theme-secondary-hover',

  // Surface styles
  surface: 'theme-surface',
  muted: 'theme-muted',
  accent: 'theme-accent',

  // Status styles
  destructive: 'theme-destructive',
  warning: 'theme-warning',
  success: 'theme-success',
  info: 'theme-info',

  // Border styles
  border: 'theme-border',
  borderPrimary: 'theme-border-primary',
  borderSecondary: 'theme-border-secondary',

  // Shadow styles
  shadowSm: 'theme-shadow-sm',
  shadowMd: 'theme-shadow-md',
  shadowLg: 'theme-shadow-lg',
  shadowXl: 'theme-shadow-xl',

  // Interactive styles
  interactive: 'theme-interactive',
  interactivePrimary: 'theme-interactive-primary',
  interactiveSecondary: 'theme-interactive-secondary',

  // Focus styles
  focusRing: 'theme-focus-ring',

  // Transition
  transition: 'theme-transition',
} as const;
