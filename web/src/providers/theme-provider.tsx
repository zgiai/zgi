'use client';

import React, { createContext, useContext, useEffect, useState } from 'react';
import type { Theme } from '@/lib/theme';
import {
  applyTheme,
  initializeTheme,
  setupSystemThemeListener,
  normalizeTheme,
  type ThemeConfig,
  getThemeConfig,
  getAllThemes,
} from '@/lib/theme';

interface ThemeContextType {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  themes: ThemeConfig[];
  currentThemeConfig: ThemeConfig;
  toggleTheme: () => void;
  isDark: boolean;
}

const ThemeContext = createContext<ThemeContextType | undefined>(undefined);

interface ThemeProviderProps {
  children: React.ReactNode;
  defaultTheme?: Theme;
  storageKey?: string;
  enableSystem?: boolean;
}

export function ThemeProvider({
  children,
  defaultTheme = 'light',
  enableSystem = true,
}: ThemeProviderProps) {
  const [theme, setThemeState] = useState<Theme>(defaultTheme);

  // Initialize theme on mount
  useEffect(() => {
    const initialTheme = initializeTheme();
    setThemeState(initialTheme);

    // Setup system theme listener if enabled
    if (enableSystem) {
      const cleanup = setupSystemThemeListener();
      return cleanup;
    }
  }, [enableSystem]);

  // Listen to theme change events
  useEffect(() => {
    const handleThemeChange = (event: CustomEvent<{ theme: Theme }>) => {
      setThemeState(event.detail.theme);
    };

    window.addEventListener('theme-change', handleThemeChange as EventListener);
    return () => {
      window.removeEventListener('theme-change', handleThemeChange as EventListener);
    };
  }, []);

  const setTheme = (newTheme: Theme) => {
    const resolvedTheme = normalizeTheme(newTheme);
    applyTheme(resolvedTheme);
    setThemeState(resolvedTheme);
  };

  const toggleTheme = () => {
    setTheme('light');
  };

  const value: ThemeContextType = {
    theme,
    setTheme,
    themes: getAllThemes(),
    currentThemeConfig: getThemeConfig(theme),
    toggleTheme,
    isDark: false,
  };

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme() {
  const context = useContext(ThemeContext);
  if (context === undefined) {
    throw new Error('useTheme must be used within a ThemeProvider');
  }
  return context;
}

// Safe version of useTheme that doesn't throw during SSR or initial render
export function useSafeTheme() {
  const context = useContext(ThemeContext);

  // Return default values if context is not available
  if (context === undefined) {
    return {
      theme: 'light' as Theme,
      setTheme: () => {},
      themes: getAllThemes(),
      currentThemeConfig: getThemeConfig('light'),
      toggleTheme: () => {},
      isDark: false,
    };
  }

  return context;
}
