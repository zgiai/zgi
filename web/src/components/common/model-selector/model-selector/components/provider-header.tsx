'use client';

import { memo } from 'react';
import { ChevronDown } from 'lucide-react';
import { cn } from '@/lib/utils';
import { ProviderIcon } from '@/components/common/provider-icon';

export interface ProviderHeaderProps {
  providerId: string;
  providerLabel: string;
  modelCount: number;
  modelCountText: string;
  isCollapsed: boolean;
  collapsible: boolean;
  onToggle: (providerId: string) => void;
}

// Provider group header component
export const ProviderHeader = memo(function ProviderHeader({
  providerId,
  providerLabel,
  modelCount,
  modelCountText,
  isCollapsed,
  collapsible,
  onToggle,
}: ProviderHeaderProps) {
  return (
    <div
      className={cn(
        'h-9 flex items-center px-2 cursor-pointer hover:bg-accent/50 rounded-sm mx-1',
        'transition-colors duration-150'
      )}
      onMouseDown={e => {
        e.preventDefault();
        e.stopPropagation();
        if (collapsible) {
          onToggle(providerId);
        }
      }}
    >
      <div className="flex items-center flex-1 gap-2">
        <div className="flex-1 py-0 text-sm font-medium cursor-pointer flex items-center">
          <div className="inline-flex items-center gap-2">
            <ProviderIcon provider={providerId} size={20} />
            {providerLabel}
          </div>
          <span className="text-xs text-muted-foreground ml-2 font-normal">
            ({modelCount} {modelCountText})
          </span>
        </div>
        {collapsible && (
          <div className="shrink-0">
            <ChevronDown
              className={cn('h-3 w-3 text-muted-foreground', isCollapsed && 'rotate-90')}
            />
          </div>
        )}
      </div>
    </div>
  );
});
