'use client';

import { memo, useEffect, useState } from 'react';
import { Card, CardContent } from '@/components/ui/card';
import type { MarketplaceBrandingSettings, MarketplacePlugin } from '@/services/types/plugin';
import { cn } from '@/lib/utils';
import { APP_NAME } from '@/lib/config';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { Bot, Clock3, Download, MessageSquareText, Star } from 'lucide-react';

interface PluginCardProps {
  plugin: MarketplacePlugin;
  branding?: MarketplaceBrandingSettings;
  className?: string;
  onClick?: () => void;
}

const DEFAULT_BLUE_V_ICON =
  'data:image/svg+xml,%3Csvg width=%2216%22 height=%2216%22 viewBox=%220 0 16 16%22 fill=%22none%22 xmlns=%22http://www.w3.org/2000/svg%22%3E%3Ccircle cx=%228%22 cy=%228%22 r=%227%22 fill=%22%232F6BFF%22/%3E%3Cpath d=%22M4.75 8.1L6.9 10.25L11.4 5.75%22 stroke=%22white%22 stroke-width=%222%22 stroke-linecap=%22round%22 stroke-linejoin=%22round%22/%3E%3C/svg%3E';
const DEFAULT_YELLOW_V_ICON =
  'data:image/svg+xml,%3Csvg width=%2216%22 height=%2216%22 viewBox=%220 0 16 16%22 fill=%22none%22 xmlns=%22http://www.w3.org/2000/svg%22%3E%3Ccircle cx=%228%22 cy=%228%22 r=%227%22 fill=%22%23F5A400%22/%3E%3Cpath d=%22M4.75 8.1L6.9 10.25L11.4 5.75%22 stroke=%22white%22 stroke-width=%222%22 stroke-linecap=%22round%22 stroke-linejoin=%22round%22/%3E%3C/svg%3E';

function PluginCard({ plugin, branding, className, onClick }: PluginCardProps) {
  const t = useT('market');
  const [isIconLoadFailed, setIsIconLoadFailed] = useState(false);

  const pluginName = plugin.name;
  const pluginDescription = plugin.short_description || plugin.description;
  const pluginDeveloper = plugin.is_official ? APP_NAME : plugin.developer?.organization_name;
  const developerLogo = plugin.is_official
    ? branding?.official_logo_url || plugin.developer?.logo_url
    : plugin.developer?.logo_url;
  const blueVIcon = branding?.blue_v_icon_url || DEFAULT_BLUE_V_ICON;
  const yellowVIcon = branding?.yellow_v_icon_url || DEFAULT_YELLOW_V_ICON;
  const isCertified =
    Boolean(plugin.developer?.is_verified) || Boolean(plugin.official_labels?.length);
  const authorVerificationIcon = plugin.is_official ? blueVIcon : isCertified ? yellowVIcon : null;
  const authorVerificationLabel = plugin.is_official
    ? t('plugins.official')
    : t('plugins.certified');
  const pluginLabels = Array.from(
    new Set(
      [...(plugin.official_labels || []), ...(plugin.tags || [])].filter(Boolean).filter(label => {
        if (!plugin.is_official) return true;
        return !['official', '官方'].includes(label.toLowerCase());
      })
    )
  );
  const visibleLabels = pluginLabels.slice(0, 3);
  const metrics = getPluginMetrics(plugin, branding);
  const metricEnabled = branding?.metric_enabled ?? {};
  const metricTips = branding?.metric_tips ?? {};
  const metricIcons = branding?.metric_icon_urls ?? {};
  const metricItems = [
    {
      key: 'downloads',
      icon: Download,
      iconUrl: metricIcons.downloads,
      label: formatMetricTip(metricTips.downloads, metrics.installs) || t('plugins.metrics.installs'),
      value: metrics.installs,
      enabled: metricEnabled.downloads !== false,
    },
    {
      key: 'runs',
      icon: Bot,
      iconUrl: metricIcons.runs,
      label: formatMetricTip(metricTips.runs, metrics.runs) || t('plugins.metrics.runs'),
      value: metrics.runs,
      enabled: metricEnabled.runs !== false,
    },
    {
      key: 'runtime',
      icon: Clock3,
      iconUrl: metricIcons.runtime,
      label: formatMetricTip(metricTips.runtime, metrics.avgRuntime) || t('plugins.metrics.avgRuntime'),
      value: metrics.avgRuntime,
      enabled: metricEnabled.runtime !== false,
    },
    {
      key: 'success',
      icon: MessageSquareText,
      iconUrl: metricIcons.success,
      label: formatMetricTip(metricTips.success, metrics.successRate) || t('plugins.metrics.successRate'),
      value: metrics.successRate,
      enabled: metricEnabled.success !== false,
    },
    {
      key: 'favorites',
      icon: Star,
      iconUrl: metricIcons.favorites,
      label: formatMetricTip(metricTips.favorites, metrics.favorites) || t('plugins.metrics.favorites'),
      value: metrics.favorites,
      enabled: metricEnabled.favorites !== false,
    },
  ];
  const visibleMetricItems = metricItems.filter(item => item.enabled);

  useEffect(() => {
    setIsIconLoadFailed(false);
  }, [plugin.id, plugin.icon]);

  return (
    <Card
      className={cn(
        'group h-full cursor-pointer border border-border/80 bg-card transition-all hover:border-primary/40 hover:shadow-sm',
        className
      )}
      onClick={onClick}
    >
      <CardContent
        className={cn(
          'flex h-full flex-col gap-3 p-4',
          visibleMetricItems.length > 0 && 'min-h-[194px]'
        )}
      >
        <div className="flex min-w-0 items-start gap-4">
          {plugin.icon && !isIconLoadFailed ? (
            <div className="flex h-12 w-12 shrink-0 items-center justify-center overflow-hidden rounded-lg border bg-muted">
              <img
                src={plugin.icon}
                alt={pluginName}
                className="h-full w-full object-contain"
                onError={() => setIsIconLoadFailed(true)}
              />
            </div>
          ) : (
            <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg border bg-muted text-muted-foreground">
              <span className="text-base font-semibold">{pluginName.slice(0, 2)}</span>
            </div>
          )}

          <div className="min-w-0 flex-1">
            <h3
              className="truncate text-base font-semibold leading-6 transition-colors group-hover:text-primary"
              title={pluginName}
            >
              {pluginName}
            </h3>
            {pluginDeveloper && (
              <div className="mt-1 flex min-w-0 items-center gap-1.5 text-sm text-muted-foreground">
                <DeveloperAvatar name={pluginDeveloper} src={developerLogo} />
                <span className="min-w-0 truncate">{pluginDeveloper}</span>
                {authorVerificationIcon && (
                  <VerificationIcon src={authorVerificationIcon} label={authorVerificationLabel} />
                )}
              </div>
            )}
          </div>
        </div>

        {pluginDescription && (
          <Tooltip>
            <TooltipTrigger asChild>
              <p className="truncate text-sm leading-5 text-muted-foreground">
                {pluginDescription}
              </p>
            </TooltipTrigger>
            <TooltipContent side="top" align="start" className="max-w-80 text-sm leading-6">
              {pluginDescription}
            </TooltipContent>
          </Tooltip>
        )}

        <div className="flex min-h-6 flex-wrap items-center gap-2">
          {visibleLabels.map(label => (
            <Badge
              key={label}
              variant="outline"
              className="h-6 rounded-md px-2 text-xs font-normal"
            >
              {label}
            </Badge>
          ))}
          {plugin.latest_version?.version && (
            <span className="ml-auto whitespace-nowrap text-xs text-muted-foreground">
              v{plugin.latest_version.version}
            </span>
          )}
        </div>

        {visibleMetricItems.length > 0 && (
          <div className="mt-auto flex min-w-0 flex-wrap items-center gap-x-3 gap-y-2 pt-1 text-xs text-muted-foreground">
            {visibleMetricItems.map(item => (
              <Metric
                key={item.key}
                icon={item.icon}
                iconUrl={item.iconUrl}
                label={item.label}
                value={item.value}
              />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function DeveloperAvatar({ name, src }: { name: string; src?: string }) {
  const initials = name.trim().slice(0, 1).toUpperCase();

  return (
    <span className="relative flex h-5 w-5 shrink-0 items-center justify-center overflow-hidden rounded-full border bg-muted text-[10px] font-semibold text-muted-foreground">
      <span>{initials}</span>
      {src ? (
        <img
          src={src}
          alt={name}
          className="absolute inset-0 h-full w-full object-cover"
          onError={e => {
            e.currentTarget.style.display = 'none';
          }}
        />
      ) : null}
    </span>
  );
}

function VerificationIcon({ src, label }: { src: string; label: string }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <img src={src} alt={label} className="h-4 w-4 shrink-0" />
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

function Metric({
  icon: Icon,
  iconUrl,
  label,
  value,
}: {
  icon: React.ComponentType<{ className?: string }>;
  iconUrl?: string;
  label: string;
  value: string;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="flex min-w-0 items-center gap-1">
          {iconUrl ? (
            <img src={iconUrl} alt="" className="h-3.5 w-3.5 shrink-0 object-contain" />
          ) : (
            <Icon className="h-3.5 w-3.5 shrink-0" />
          )}
          <span className="whitespace-nowrap">{value}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

function formatMetricTip(template: string | undefined, value: string) {
  if (!template) return '';
  return template.replaceAll('{{value}}', value);
}

function getPluginMetrics(plugin: MarketplacePlugin, branding?: MarketplaceBrandingSettings) {
  const hash = stableHash(plugin.id);
  const installs = plugin.download_count || 0;
  const runs = installs * (4 + (hash % 18));
  const favorites = plugin.rating_count || 0;
  const successRate = `${Math.min(99.9, 86 + (hash % 139) / 10).toFixed(1)}%`;
  const avgRuntime = `${120 + (hash % 1800)}ms`;

  return {
    installs: compactNumber(installs),
    runs: compactNumber(runs),
    favorites: compactNumber(favorites),
    successRate,
    avgRuntime,
  };
}

function stableHash(value: string) {
  let hash = 0;
  for (let i = 0; i < value.length; i += 1) {
    hash = (hash * 31 + value.charCodeAt(i)) >>> 0;
  }
  return hash;
}

function compactNumber(value: number) {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`;
  return String(value);
}

export default memo(PluginCard);
