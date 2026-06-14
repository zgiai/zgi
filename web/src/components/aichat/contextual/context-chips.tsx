'use client';

import { X } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import type { AIChatContextItem } from './types';

interface AIChatContextChipsProps {
  items: AIChatContextItem[];
  maxVisible?: number;
  className?: string;
  onClear?: () => void;
}

function contextTypeLabel(type: AIChatContextItem['type']) {
  switch (type) {
    case 'agent':
      return 'Agent';
    case 'workflow':
      return 'Workflow';
    case 'file':
      return 'File';
    case 'task':
      return 'Task';
    case 'dataset':
      return 'Dataset';
    case 'database':
      return 'Database';
    case 'log':
      return 'Log';
    case 'selection':
      return 'Selection';
    case 'page':
      return 'Page';
    default:
      return 'Context';
  }
}

export function AIChatContextChips({
  items,
  maxVisible = 4,
  className,
  onClear,
}: AIChatContextChipsProps) {
  const visibleItems = items.slice(0, maxVisible);
  const hiddenCount = Math.max(0, items.length - visibleItems.length);

  if (items.length === 0) {
    return null;
  }

  return (
    <div className={cn('flex min-w-0 flex-wrap items-center gap-1.5', className)}>
      {visibleItems.map(item => (
        <Badge key={`${item.type}:${item.id}`} variant="outline" className="max-w-[220px] gap-1">
          <span className="shrink-0 text-muted-foreground">{contextTypeLabel(item.type)}</span>
          <span className="truncate">{item.title}</span>
        </Badge>
      ))}
      {hiddenCount > 0 ? <Badge variant="subtle">+{hiddenCount}</Badge> : null}
      {onClear ? (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          isIcon
          className="size-6 rounded-full text-muted-foreground"
          onClick={onClear}
        >
          <X className="size-3.5" />
          <span className="sr-only">Clear AIChat context</span>
        </Button>
      ) : null}
    </div>
  );
}
