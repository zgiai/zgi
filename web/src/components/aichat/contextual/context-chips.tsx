'use client';

import { X } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import type { AIChatCapabilityRisk, AIChatContextItem } from './types';

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

const RISK_RANK: Record<AIChatCapabilityRisk, number> = {
  low: 1,
  medium: 2,
  high: 3,
};

function getHighestCapabilityRisk(item: AIChatContextItem): AIChatCapabilityRisk | null {
  return (
    (item.capabilities ?? [])
      .map(capability => capability.risk)
      .sort((left, right) => RISK_RANK[right] - RISK_RANK[left])[0] ?? null
  );
}

function capabilitySummary(item: AIChatContextItem): string | null {
  const capabilityCount = item.capabilities?.length ?? 0;
  if (capabilityCount === 0) return null;
  const highestRisk = getHighestCapabilityRisk(item);
  return `${capabilityCount} cap${capabilityCount === 1 ? '' : 's'}${
    highestRisk ? `/${highestRisk}` : ''
  }`;
}

function capabilityRiskClass(risk: AIChatCapabilityRisk | null) {
  switch (risk) {
    case 'high':
      return 'text-destructive';
    case 'medium':
      return 'text-amber-600 dark:text-amber-400';
    default:
      return 'text-muted-foreground';
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
      {visibleItems.map(item => {
        const summary = capabilitySummary(item);
        const highestRisk = getHighestCapabilityRisk(item);
        return (
          <Badge key={`${item.type}:${item.id}`} variant="outline" className="max-w-[280px] gap-1">
            <span className="shrink-0 text-muted-foreground">{contextTypeLabel(item.type)}</span>
            <span className="truncate">{item.title}</span>
            {summary ? (
              <span className={cn('shrink-0 text-[11px]', capabilityRiskClass(highestRisk))}>
                {summary}
              </span>
            ) : null}
          </Badge>
        );
      })}
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
