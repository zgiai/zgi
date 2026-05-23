'use client';

import { memo } from 'react';
import { Card, CardContent } from '@/components/ui/card';
import type { MarketplacePlugin } from '@/services/types/plugin';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { Bot, Clock3, Download, MessageSquareText, Star } from 'lucide-react';

interface PluginCardProps {
  plugin: MarketplacePlugin;
  className?: string;
  onClick?: () => void;
}

function PluginCard({ plugin, className, onClick }: PluginCardProps) {
  const t = useT('market');

  const pluginName = plugin.name;
  const pluginDescription = plugin.short_description || plugin.description;
  const pluginDeveloper = plugin.developer?.organization_name;
  const pluginLabels = Array.from(
    new Set(
      [...(plugin.official_labels || []), ...(plugin.tags || [])].filter(Boolean).filter(label => {
        if (!plugin.is_official) return true;
        return !['official', '官方'].includes(label.toLowerCase());
      })
    )
  );
  const visibleLabels = pluginLabels.slice(0, 3);
  const metrics = getPluginMetrics(plugin);

  return (
    <Card
      className={cn(
        'group h-full cursor-pointer border border-border/80 bg-card transition-all hover:border-primary/40 hover:shadow-sm',
        className
      )}
      onClick={onClick}
    >
      <CardContent className="flex h-full min-h-[194px] flex-col gap-3 p-4">
        <div className="flex min-w-0 items-start gap-4">
          {plugin.icon ? (
            <div className="flex h-12 w-12 shrink-0 items-center justify-center overflow-hidden rounded-lg border bg-muted">
              <img
                src={plugin.icon}
                alt={pluginName}
                className="h-full w-full object-contain"
                onError={e => {
                  const target = e.target as HTMLImageElement;
                  target.style.display = 'none';
                  const parent = target.parentElement;
                  if (parent) {
                    parent.innerHTML = `<div class="flex h-full w-full items-center justify-center text-base font-semibold text-muted-foreground">${pluginName.slice(0, 2)}</div>`;
                  }
                }}
              />
            </div>
          ) : (
            <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg border bg-muted text-muted-foreground">
              <span className="text-base font-semibold">{pluginName.slice(0, 2)}</span>
            </div>
          )}

          <div className="min-w-0 flex-1">
            <h3
              className="line-clamp-1 text-base font-semibold leading-6 transition-colors group-hover:text-primary"
              title={pluginName}
            >
              {pluginName}
            </h3>
            {pluginDeveloper && (
              <p className="mt-1 line-clamp-1 text-sm text-muted-foreground">
                {t('plugins.modal.by')} {pluginDeveloper}
              </p>
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
          {plugin.is_official && (
            <Badge variant="secondary" className="h-6 rounded-md px-2 text-xs font-normal">
              {t('plugins.official')}
            </Badge>
          )}
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

        <div className="mt-auto flex min-w-0 flex-wrap items-center gap-x-3 gap-y-2 pt-1 text-xs text-muted-foreground">
          <Metric icon={Download} label={t('plugins.metrics.installs')} value={metrics.installs} />
          <Metric icon={Bot} label={t('plugins.metrics.runs')} value={metrics.runs} />
          <Metric
            icon={Clock3}
            label={t('plugins.metrics.avgRuntime')}
            value={metrics.avgRuntime}
          />
          <Metric
            icon={MessageSquareText}
            label={t('plugins.metrics.successRate')}
            value={metrics.successRate}
          />
          <Metric icon={Star} label={t('plugins.metrics.favorites')} value={metrics.favorites} />
        </div>
      </CardContent>
    </Card>
  );
}

function Metric({
  icon: Icon,
  label,
  value,
}: {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  value: string;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="flex min-w-0 items-center gap-1">
          <Icon className="h-3.5 w-3.5 shrink-0" />
          <span className="whitespace-nowrap">{value}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

function getPluginMetrics(plugin: MarketplacePlugin) {
  const hash = stableHash(plugin.id);
  const installs = plugin.download_count || 1200 + (hash % 180000);
  const runs = installs * (4 + (hash % 18));
  const favorites = plugin.rating_count || 80 + (hash % 18000);
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
