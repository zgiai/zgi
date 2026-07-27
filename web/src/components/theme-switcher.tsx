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
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
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

export function ThemeSwitcherSubmenu() {
  const t = useT();
  const { theme, setTheme, themes } = useSafeTheme();

  if (!ENABLE_THEME_SWITCH) {
    return null;
  }

  return (
    <DropdownMenuSub>
      <DropdownMenuSubTrigger className="gap-2">
        <Palette className="h-4 w-4" />
        <span>{t('settings.themes.chooseTheme')}</span>
      </DropdownMenuSubTrigger>
      <DropdownMenuSubContent sideOffset={6} className="w-52">
        {THEME_OPTIONS.map(option => {
          const preview = themes.find(themeConfig => themeConfig.name === option.key)?.preview;

          return (
            <DropdownMenuItem
              key={option.key}
              onClick={() => setTheme(option.key)}
              className="gap-2"
            >
              <span className="flex shrink-0 items-center gap-1" aria-hidden="true">
                <span
                  className="h-3 w-3 rounded-full border border-border/70"
                  style={{ backgroundColor: preview?.primary }}
                />
                <span
                  className="h-3 w-3 rounded-full border border-border/70"
                  style={{ backgroundColor: preview?.secondary }}
                />
                <span
                  className="h-3 w-3 rounded-full border border-border/70"
                  style={{ backgroundColor: preview?.background }}
                />
              </span>
              <span className="min-w-0 flex-1 truncate">{t(option.labelKey)}</span>
              {theme === option.key && <Check className="h-4 w-4 shrink-0 text-primary" />}
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuSubContent>
    </DropdownMenuSub>
  );
}

// Quick toggle is hidden while dark mode is temporarily disabled.
export function QuickThemeToggle({ className }: { className?: string }) {
  void className;
  return null;
}
