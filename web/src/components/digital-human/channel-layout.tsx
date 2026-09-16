'use client';

import type { ReactNode } from 'react';
import { Cable, MessageSquare, Plus, RefreshCw, Send, type LucideIcon } from 'lucide-react';
import { Button } from '@/components/ui/button';

export function ChannelHeader({
  title,
  description,
  canManage,
  onRefresh,
  onAdd,
  refreshLabel,
  addLabel,
}: {
  title: string;
  description: string;
  canManage: boolean;
  onRefresh: () => void;
  onAdd: () => void;
  refreshLabel: string;
  addLabel: string;
}) {
  return (
    <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
      <div className="space-y-1">
        <h2 className="text-2xl font-semibold tracking-tight">{title}</h2>
        <p className="max-w-3xl text-sm text-muted-foreground">{description}</p>
      </div>
      <div className="flex gap-2">
        <Button variant="outline" size="sm" onClick={onRefresh}>
          <RefreshCw className="size-4" />
          {refreshLabel}
        </Button>
        {canManage ? (
          <Button size="sm" onClick={onAdd}>
            <Plus className="size-4" />
            {addLabel}
          </Button>
        ) : null}
      </div>
    </div>
  );
}

export function ChannelMetrics({
  activeInstallations,
  recentEvents,
  pendingDeliveries,
  canViewActivity,
  labels,
}: {
  activeInstallations: number;
  recentEvents: number;
  pendingDeliveries: number;
  canViewActivity: boolean;
  labels: { installations: string; recentEvents: string; pending: string };
}) {
  return (
    <div className="grid gap-3 sm:grid-cols-3">
      <MetricCard icon={Cable} label={labels.installations} value={String(activeInstallations)} />
      <MetricCard
        icon={MessageSquare}
        label={labels.recentEvents}
        value={canViewActivity ? String(recentEvents) : '—'}
      />
      <MetricCard
        icon={Send}
        label={labels.pending}
        value={canViewActivity ? String(pendingDeliveries) : '—'}
      />
    </div>
  );
}

function MetricCard({
  icon: Icon,
  label,
  value,
}: {
  icon: LucideIcon;
  label: string;
  value: string;
}) {
  return (
    <div className="flex items-center gap-3 rounded-xl border bg-card px-4 py-3 shadow-sm">
      <span className="flex size-9 items-center justify-center rounded-lg bg-primary/10 text-primary">
        <Icon className="size-4" />
      </span>
      <div>
        <p className="text-xs text-muted-foreground">{label}</p>
        <p className="text-lg font-semibold tabular-nums">{value}</p>
      </div>
    </div>
  );
}

export function ChannelSection({ children }: { children: ReactNode }) {
  return <section className="min-w-0">{children}</section>;
}
