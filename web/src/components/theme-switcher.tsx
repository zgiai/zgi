'use client';

import React from 'react';
import { useSafeTheme } from '@/providers/theme-provider';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Palette, Sun, Monitor, Check, Sparkles } from 'lucide-react';
import { type Theme } from '@/lib/theme';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import { ENABLE_THEME_SWITCH } from '@/lib/config';

interface ThemeSwitcherProps {
  className?: string;
  showLabel?: boolean;
  variant?: 'icon' | 'button' | 'preview';
  hidePreviewSwatch?: boolean;
}

interface ThemeOption {
  key: Theme;
  labelKey:
    | 'settings.themes.light'
    | 'settings.themes.tech-blue'
    | 'settings.themes.graphite-cyan'
    | 'settings.themes.emerald'
    | 'settings.themes.violet'
    | 'settings.themes.warm-orange';
  descKey:
    | 'settings.themes.lightDesc'
    | 'settings.themes.tech-blueDesc'
    | 'settings.themes.graphite-cyanDesc'
    | 'settings.themes.emeraldDesc'
    | 'settings.themes.violetDesc'
    | 'settings.themes.warm-orangeDesc';
  icon: React.ReactNode;
}

const THEME_OPTIONS: ThemeOption[] = [
  {
    key: 'light',
    labelKey: 'settings.themes.light',
    descKey: 'settings.themes.lightDesc',
    icon: <Sun className="h-4 w-4" />,
  },
  {
    key: 'graphite-cyan',
    labelKey: 'settings.themes.graphite-cyan',
    descKey: 'settings.themes.graphite-cyanDesc',
    icon: <Sparkles className="h-4 w-4" />,
  },
  {
    key: 'emerald',
    labelKey: 'settings.themes.emerald',
    descKey: 'settings.themes.emeraldDesc',
    icon: <Sparkles className="h-4 w-4" />,
  },
  {
    key: 'violet',
    labelKey: 'settings.themes.violet',
    descKey: 'settings.themes.violetDesc',
    icon: <Sparkles className="h-4 w-4" />,
  },
  {
    key: 'warm-orange',
    labelKey: 'settings.themes.warm-orange',
    descKey: 'settings.themes.warm-orangeDesc',
    icon: <Sparkles className="h-4 w-4" />,
  },
  {
    key: 'tech-blue',
    labelKey: 'settings.themes.tech-blue',
    descKey: 'settings.themes.tech-blueDesc',
    icon: <Sparkles className="h-4 w-4" />,
  },
];

export function ThemeSwitcher({
  className,
  showLabel = false,
  variant = 'icon',
  hidePreviewSwatch = false,
}: ThemeSwitcherProps) {
  const t = useT();
  const { theme, setTheme, currentThemeConfig } = useSafeTheme();

  // Hide component when theme switching is disabled
  if (!ENABLE_THEME_SWITCH) {
    return null;
  }

  const currentOption = THEME_OPTIONS.find(opt => opt.key === theme);

  const renderTrigger = () => {
    if (variant === 'button') {
      return (
        <Button variant="outline" className={cn('gap-2', className)}>
          <Palette className="h-4 w-4" />
          {showLabel && currentOption && t(currentOption.labelKey)}
        </Button>
      );
    }

    if (variant === 'preview') {
      return (
        <Button variant="outline" className={cn('gap-2 min-w-[120px] justify-start', className)}>
          <div className="flex items-center gap-2">
            {!hidePreviewSwatch && (
              <div
                className="w-4 h-4 rounded-full border"
                style={{
                  backgroundColor: currentThemeConfig.preview?.background,
                  borderColor: currentThemeConfig.preview?.primary,
                }}
              />
            )}
            {currentOption && t(currentOption.labelKey)}
          </div>
        </Button>
      );
    }

    return (
      <Button
        variant="ghost"
        isIcon
        className={cn('theme-interactive', className)}
        aria-label={t('settings.themes.toggleTheme')}
      >
        {currentOption?.icon}
      </Button>
    );
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>{renderTrigger()}</DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <DropdownMenuLabel className="flex items-center gap-2">
          <Palette className="h-4 w-4" />
          {t('settings.themes.chooseTheme')}
        </DropdownMenuLabel>
        <DropdownMenuSeparator />

        {THEME_OPTIONS.map(option => (
          <DropdownMenuItem
            key={option.key}
            onClick={() => setTheme(option.key)}
            className="flex items-center justify-between cursor-pointer"
          >
            <div className="flex items-center gap-3">
              {option.icon}
              <div>
                <div className="font-medium">{t(option.labelKey)}</div>
                <div className="text-xs text-muted-foreground">{t(option.descKey)}</div>
              </div>
            </div>
            {theme === option.key && <Check className="h-4 w-4 text-primary" />}
          </DropdownMenuItem>
        ))}

        <DropdownMenuSeparator />
        <DropdownMenuItem className="text-xs text-muted-foreground justify-center">
          <Monitor className="h-3 w-3 mr-1" />
          {t('settings.themes.systemPreference')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

// Quick toggle is hidden while dark mode is temporarily disabled.
export function QuickThemeToggle({ className }: { className?: string }) {
  void className;
  return null;
}

// Theme preview component for settings
export function ThemePreview({
  themeName,
  isSelected,
  onSelect,
  className,
  displayName,
  description,
}: {
  themeName: Theme;
  isSelected: boolean;
  onSelect: (theme: Theme) => void;
  className?: string;
  displayName?: string;
  description?: string;
}) {
  const { themes } = useSafeTheme();
  const themeConfig = themes.find(t => t.name === themeName);

  if (!themeConfig?.preview) {
    return null;
  }

  return (
    <button
      onClick={() => onSelect(themeName)}
      className={cn(
        'relative w-full rounded-lg border p-4 text-left transition-colors duration-150',
        'focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2',
        'bg-card hover:bg-muted/40',
        isSelected
          ? 'border-primary/70 bg-primary/[0.025]'
          : 'border-border hover:border-primary/30',
        className
      )}
    >
      <div className="space-y-3">
        <div className="flex gap-2">
          <div
            className="h-4 w-4 rounded-full border border-border/60"
            style={{ backgroundColor: themeConfig.preview.primary }}
          />
          <div
            className="h-4 w-4 rounded-full border border-border/60"
            style={{ backgroundColor: themeConfig.preview.secondary }}
          />
          <div
            className="h-4 w-4 rounded-full border border-border/60"
            style={{ backgroundColor: themeConfig.preview.background }}
          />
        </div>

        <div>
          <div className="text-sm font-medium leading-5 text-foreground">{displayName}</div>
          {description && (
            <div className="mt-1 text-xs leading-5 text-muted-foreground">{description}</div>
          )}
        </div>
      </div>

      {isSelected && (
        <div className="absolute right-3 top-3 flex h-5 w-5 items-center justify-center rounded-full bg-primary text-primary-foreground">
          <Check className="h-3 w-3" />
        </div>
      )}
    </button>
  );
}
