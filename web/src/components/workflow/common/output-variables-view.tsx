'use client';

import React from 'react';
import { Badge } from '@/components/ui/badge';
import { useT } from '@/i18n';
import { ChevronDown } from 'lucide-react';
import { cn } from '@/lib/utils';

export interface OutputVariableViewItem {
  name: string;
  type: string;
  description?: string;
}

export interface OutputVariablesViewProps {
  variables?: OutputVariableViewItem[];
  className?: string;
  title?: string;
  emptyText?: string;
  variant?: 'panel' | 'compact';
  maxItems?: number;
  showCount?: boolean;
}

const OutputVariablesView: React.FC<OutputVariablesViewProps> = ({
  variables,
  className,
  title,
  emptyText,
  variant = 'panel',
  maxItems = 3,
  showCount = true,
}) => {
  const t = useT('nodes');
  const items = Array.isArray(variables) ? variables : [];
  const label = React.useMemo(() => {
    if (title) return title;
    try {
      return t('common.outputVariables');
    } catch {
      return 'Output Variables';
    }
  }, [t, title]);
  const emptyLabel = React.useMemo(() => {
    if (emptyText) return emptyText;
    return t('common.noVariables');
  }, [emptyText, t]);

  const [open, setOpen] = React.useState<boolean>(variant === 'compact');

  React.useEffect(() => {
    setOpen(variant === 'compact');
  }, [variant]);

  if (variant === 'compact') {
    if (items.length === 0) return null;

    const visibleItems = items.slice(0, Math.max(1, maxItems));
    const hiddenCount = Math.max(0, items.length - visibleItems.length);

    return (
      <div className={cn('border-t pt-2 space-y-1.5', className)}>
        <div className="flex items-center gap-2 text-xs font-medium text-primary">
          <span className="truncate">{label}</span>
          {showCount ? (
            <span className="ml-auto text-[10px] text-muted-foreground">{items.length}</span>
          ) : null}
        </div>
        <div className="space-y-1">
          {visibleItems.map(variable => (
            <div
              key={variable.name}
              className="flex items-center justify-between gap-2 rounded-md border bg-background/70 px-2 py-1 text-xs"
            >
              <span className="truncate font-medium">{variable.name}</span>
              <span className="truncate text-muted-foreground">{variable.type}</span>
            </div>
          ))}
          {hiddenCount > 0 ? (
            <div className="px-1 text-[11px] text-muted-foreground">
              +{hiddenCount}
            </div>
          ) : null}
        </div>
      </div>
    );
  }

  return (
    <div className={cn('border-t pt-2', className)}>
      <div
        className="group flex items-center gap-1 w-full select-none rounded-md py-1 transition-colors cursor-pointer"
        role="button"
        aria-expanded={open}
        tabIndex={0}
        onClick={() => setOpen(o => !o)}
        onKeyDown={e => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            setOpen(o => !o);
          }
        }}
      >
        <ChevronDown
          className={`h-4 w-4 transition-transform ${open ? 'rotate-0' : '-rotate-90'}`}
        />
        <div className="text-sm font-medium text-foreground">{label}</div>
        {showCount ? (
          <div className="ml-auto text-xs text-muted-foreground px-1">{items.length}</div>
        ) : null}
      </div>
      <div className={`${open ? 'mt-2' : 'mt-2 hidden'}`}>
        {items.length === 0 ? (
          <div className="text-xs text-muted-foreground px-1">{emptyLabel}</div>
        ) : (
          <div className="space-y-1">
            {items.map(v => (
              <div
                key={v.name}
                className={cn(
                  'flex items-start gap-3 py-1.5 px-2 rounded-md bg-accent transition-colors'
                )}
              >
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <div className="text-sm font-medium truncate">{v.name}</div>
                    <Badge className="px-1.5 py-0 text-xs font-normal">{v.type}</Badge>
                  </div>
                  {v.description ? (
                    <div className="mt-0.5 text-xs text-muted-foreground leading-relaxed">
                      {v.description}
                    </div>
                  ) : null}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};

export default OutputVariablesView;
