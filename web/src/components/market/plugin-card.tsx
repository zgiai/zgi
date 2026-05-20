'use client';

import { memo } from 'react';
import { Card, CardContent } from '@/components/ui/card';
import type { MarketplacePlugin } from '@/services/types/plugin';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';

interface PluginCardProps {
  plugin: MarketplacePlugin;
  className?: string;
  onClick?: () => void;
}

function PluginCard({ plugin, className, onClick }: PluginCardProps) {
  const t = useT('market');

  const pluginName = plugin.name;
  const pluginDescription = plugin.description;
  const pluginDeveloper = plugin.developer?.organization_name;

  return (
    <Card
      className={cn(
        'hover:shadow-md transition-shadow h-full flex flex-col border border-border cursor-pointer relative min-h-[120px]',
        className
      )}
      onClick={onClick}
    >
      <CardContent className="p-4 sm:p-5 space-y-3 flex-1 flex flex-col">
        {/* Icon and Title */}
        <div className="flex items-start gap-3">
          {plugin.icon ? (
            <div className="w-10 h-10 sm:w-12 sm:h-12 flex items-center justify-center shrink-0 rounded-lg bg-muted overflow-hidden">
              <img
                src={plugin.icon}
                alt={pluginName}
                className="w-full h-full object-contain"
                onError={e => {
                  const target = e.target as HTMLImageElement;
                  target.style.display = 'none';
                  const parent = target.parentElement;
                  if (parent) {
                    parent.innerHTML = `<div class="w-full h-full flex items-center justify-center text-lg font-semibold text-muted-foreground">${pluginName}</div>`;
                  }
                }}
              />
            </div>
          ) : (
            <div className="w-10 h-10 sm:w-12 sm:h-12 flex items-center justify-center shrink-0 rounded-lg bg-muted text-muted-foreground">
              <span className="text-lg font-semibold">{pluginName.slice(0, 2)}</span>
            </div>
          )}
          <div className="flex-1 min-w-0">
            <h3 className="font-semibold text-base sm:text-lg line-clamp-2" title={pluginName}>
              {pluginName}
            </h3>
            {/* Description */}
            {pluginDescription && (
              <p className="text-sm text-gray-500 line-clamp-2 flex-1 mt-1 h-[40px] overflow-hidden">
                {pluginDescription}
              </p>
            )}
            {/* developer */}
            {pluginDeveloper && (
              <p className="text-sm text-muted-foreground line-clamp-3 flex-1 mt-3">
                {t('plugins.modal.by')} {pluginDeveloper}
              </p>
            )}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

export default memo(PluginCard);
