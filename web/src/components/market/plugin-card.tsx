'use client';

import { memo } from 'react';
import { Card, CardContent } from '@/components/ui/card';
import type { MarketplacePlugin } from '@/services/types/plugin';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';

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
      [...(plugin.official_labels || []), ...(plugin.tags || [])]
        .filter(Boolean)
        .filter(label => {
          if (!plugin.is_official) return true;
          return !['official', '官方'].includes(label.toLowerCase());
        })
    )
  );
  const visibleLabels = pluginLabels.slice(0, 3);

  return (
    <Card
      className={cn(
        'group h-full cursor-pointer border border-border/80 bg-card transition-all hover:border-primary/40 hover:shadow-sm',
        className
      )}
      onClick={onClick}
    >
      <CardContent className="flex h-full min-h-[176px] flex-col gap-4 p-4 sm:p-5">
        <div className="flex min-w-0 items-start gap-4">
          {plugin.icon ? (
            <div className="flex h-14 w-14 shrink-0 items-center justify-center overflow-hidden rounded-xl border bg-muted">
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
            <div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-xl border bg-muted text-muted-foreground">
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
          <p className="line-clamp-2 min-h-[40px] text-sm leading-5 text-muted-foreground">
            {pluginDescription}
          </p>
        )}

        <div className="mt-auto flex min-h-6 flex-wrap items-center gap-2">
          {plugin.is_official && (
            <Badge variant="secondary" className="h-6 rounded-md px-2 text-xs font-normal">
              {t('plugins.official')}
            </Badge>
          )}
          {visibleLabels.map(label => (
            <Badge key={label} variant="outline" className="h-6 rounded-md px-2 text-xs font-normal">
              {label}
            </Badge>
          ))}
          {plugin.latest_version?.version && (
            <span className="ml-auto whitespace-nowrap text-xs text-muted-foreground">
              v{plugin.latest_version.version}
            </span>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

export default memo(PluginCard);
