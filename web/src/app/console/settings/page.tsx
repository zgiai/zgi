'use client';

import { useSafeTheme } from '@/providers/theme-provider';
import { ENABLE_THEME_SWITCH } from '@/lib/config';
import { PageHeader } from '@/components/page-header';
import { ThemePreview, ThemeSwitcher } from '@/components/theme-switcher';
import { LanguageSwitcher } from '@/components/common/language-switcher';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { toast } from 'sonner';
import {
  KeyRound,
  Languages,
  Palette,
  Plug,
  RefreshCw,
  Shield,
  SlidersHorizontal,
} from 'lucide-react';
import type { Theme, ThemeConfig } from '@/lib/theme';
import { useT, type SettingsKey } from '@/i18n';

const SETTING_GROUPS = [
  {
    key: 'appearance',
    icon: Palette,
    titleKey: 'settings.system.appearance',
    descKey: 'settings.system.appearanceDesc',
    statusKey: 'settings.system.active',
  },
  {
    key: 'language',
    icon: Languages,
    titleKey: 'settings.system.language',
    descKey: 'settings.system.languageDesc',
    statusKey: 'settings.system.available',
  },
  {
    key: 'integrations',
    icon: Plug,
    titleKey: 'settings.system.integrations',
    descKey: 'settings.system.integrationsDesc',
    statusKey: 'settings.system.later',
  },
  {
    key: 'security',
    icon: Shield,
    titleKey: 'settings.system.security',
    descKey: 'settings.system.securityDesc',
    statusKey: 'settings.system.later',
  },
] as const;

export default function SettingsPage() {
  const t = useT();
  const { theme, setTheme, themes, currentThemeConfig } = useSafeTheme();

  const handleThemeSelect = (selectedTheme: Theme) => {
    setTheme(selectedTheme);
    toast(t('settings.messages.themeChanged'));
  };

  const resetToDefault = () => {
    setTheme('light');
    toast(t('settings.themes.resetTheme'));
  };

  return (
    <div className="space-y-6 p-6">
      <PageHeader title={t('settings.title')} description={t('settings.pageDescription')} />

      <div className="grid gap-6 lg:grid-cols-[260px_1fr]">
        <aside className="h-fit rounded-lg border border-border bg-card p-2">
          <div className="px-3 py-2 text-xs font-medium text-muted-foreground">
            {t('settings.system.groups')}
          </div>
          <div className="space-y-1">
            {SETTING_GROUPS.map(item => {
              const Icon = item.icon;
              const isActive = item.key === 'appearance';

              return (
                <div
                  key={item.key}
                  className={`rounded-md px-3 py-2.5 transition-colors ${
                    isActive ? 'bg-muted text-foreground' : 'text-muted-foreground'
                  }`}
                >
                  <div className="flex items-start gap-3">
                    <Icon className={`mt-0.5 h-4 w-4 ${isActive ? 'text-primary' : ''}`} />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center justify-between gap-2">
                        <p className="text-sm font-medium">{t(item.titleKey as SettingsKey)}</p>
                        <span className="text-[11px] text-muted-foreground">
                          {t(item.statusKey as SettingsKey)}
                        </span>
                      </div>
                      <p className="mt-0.5 text-xs leading-5 text-muted-foreground">
                        {t(item.descKey as SettingsKey)}
                      </p>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        </aside>

        <main className="space-y-5">
          {ENABLE_THEME_SWITCH ? (
            <Card className="border-border">
              <CardHeader className="pb-4">
                <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
                  <div>
                    <CardTitle className="flex items-center gap-2 text-xl">
                      <SlidersHorizontal className="h-5 w-5 text-primary" />
                      {t('settings.system.appearance')}
                    </CardTitle>
                    <CardDescription className="mt-1">
                      {t('settings.system.appearanceLongDesc')}
                    </CardDescription>
                  </div>
                  <div className="flex items-center gap-2">
                    <ThemeSwitcher
                      variant="preview"
                      hidePreviewSwatch
                      className="justify-center border-primary bg-primary text-primary-foreground hover:bg-primary/90 hover:text-primary-foreground"
                    />
                    {theme !== 'light' && (
                      <Button
                        variant="ghost"
                        className="h-9 min-w-[120px] justify-center px-4"
                        onClick={resetToDefault}
                      >
                        {t('settings.themes.resetTheme')}
                      </Button>
                    )}
                  </div>
                </div>
              </CardHeader>
              <CardContent className="space-y-5">
                <div className="rounded-lg border border-border bg-muted/25 p-4">
                  <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
                    <div>
                      <div className="flex items-center gap-2">
                        <p className="text-sm font-medium text-muted-foreground">
                          {t('settings.themes.currentTheme')}
                        </p>
                        {theme === 'light' && (
                          <Badge variant="outline" className="h-5 rounded-full px-2 text-[11px]">
                            {t('settings.themes.default')}
                          </Badge>
                        )}
                      </div>
                      <h2 className="mt-1 text-2xl font-semibold tracking-tight">
                        {t(`settings.themes.${currentThemeConfig.name}` as SettingsKey)}
                      </h2>
                      <p className="mt-1 text-sm text-muted-foreground">
                        {t(`settings.themes.${currentThemeConfig.name}Desc` as SettingsKey)}
                      </p>
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="h-8 w-8 rounded-full border border-border bg-[var(--primary)]" />
                      <span className="h-8 w-8 rounded-full border border-border bg-[var(--secondary)]" />
                      <span className="h-8 w-8 rounded-full border border-border bg-[var(--background)]" />
                    </div>
                  </div>
                </div>

                <div>
                  <div className="mb-3 flex items-center justify-between">
                    <div>
                      <h3 className="text-sm font-medium">
                        {t('settings.themes.availableThemes')}
                      </h3>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {t('settings.themes.availableThemesDesc')}
                      </p>
                    </div>
                    <Badge variant="secondary" className="rounded-full text-xs">
                      {themes.length} {t('settings.themes.available')}
                    </Badge>
                  </div>
                  <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
                    {themes.map((themeConfig: ThemeConfig) => (
                      <ThemePreview
                        key={themeConfig.name}
                        themeName={themeConfig.name as Theme}
                        displayName={t(`settings.themes.${themeConfig.name}` as SettingsKey)}
                        description={t(`settings.themes.${themeConfig.name}Desc` as SettingsKey)}
                        isSelected={theme === themeConfig.name}
                        onSelect={handleThemeSelect}
                      />
                    ))}
                  </div>
                </div>
              </CardContent>
            </Card>
          ) : null}

          <Card className="border-border">
            <CardHeader className="pb-4">
              <CardTitle className="flex items-center gap-2 text-xl">
                <Languages className="h-5 w-5 text-primary" />
                {t('settings.system.language')}
              </CardTitle>
              <CardDescription>{t('settings.system.languageLongDesc')}</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex flex-col gap-3 rounded-lg border border-border bg-card p-4 sm:flex-row sm:items-center sm:justify-between">
                <div>
                  <p className="text-sm font-medium">{t('settings.system.interfaceLanguage')}</p>
                  <p className="mt-1 text-sm text-muted-foreground">
                    {t('settings.system.interfaceLanguageDesc')}
                  </p>
                </div>
                <LanguageSwitcher className="h-9 min-w-[120px] justify-center border-primary bg-primary px-4 text-primary-foreground hover:bg-primary/90 hover:text-primary-foreground" />
              </div>
            </CardContent>
          </Card>

          <Card className="border-border">
            <CardHeader className="pb-4">
              <CardTitle className="flex items-center gap-2 text-xl">
                <KeyRound className="h-5 w-5 text-muted-foreground" />
                {t('settings.system.advanced')}
              </CardTitle>
              <CardDescription>{t('settings.system.advancedDesc')}</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex items-center justify-between rounded-lg border border-dashed border-border p-4 text-sm text-muted-foreground">
                <span>{t('settings.system.advancedPlaceholder')}</span>
                <RefreshCw className="h-4 w-4" />
              </div>
            </CardContent>
          </Card>
        </main>
      </div>
    </div>
  );
}
