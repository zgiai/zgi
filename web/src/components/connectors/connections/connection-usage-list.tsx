'use client';

import Link from 'next/link';
import { Bot, MessageCircle, Workflow } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { ConnectorTargetBinding, ConnectorTargetType } from '@/services/types/connector';
import { connectorBindingTargetHref, sortConnectionBindings } from './connection-management-model';

const SURFACE_STYLE: Record<ConnectorTargetType, string> = {
  chat: 'border-transparent bg-primary/10 text-primary',
  agent: 'border-transparent bg-violet-500/10 text-violet-600 dark:text-violet-300',
  workflow: 'border-transparent bg-cyan-500/10 text-cyan-700 dark:text-cyan-300',
};

function SurfaceIcon({ type }: { type: ConnectorTargetType }) {
  const className = 'size-4';
  if (type === 'chat') return <MessageCircle aria-hidden="true" className={className} />;
  if (type === 'agent') return <Bot aria-hidden="true" className={className} />;
  return <Workflow aria-hidden="true" className={className} />;
}

interface ConnectionUsageListProps {
  bindings: readonly ConnectorTargetBinding[];
  className?: string;
}

export function ConnectionUsageList({ bindings, className }: ConnectionUsageListProps) {
  const t = useT('connectors');
  const sortedBindings = sortConnectionBindings(bindings);

  const surfaceLabel = (type: ConnectorTargetType) => {
    if (type === 'chat') return t('usage.surfaces.chat');
    if (type === 'agent') return t('usage.surfaces.agent');
    return t('usage.surfaces.workflow');
  };

  const statusLabel = (status: ConnectorTargetBinding['status']) => {
    if (status === 'active') return t('usage.status.active');
    if (status === 'disabled') return t('usage.status.disabled');
    return t('usage.status.needsReplacement');
  };

  return (
    <Card className={cn('h-full', className)}>
      <CardHeader>
        <div className="flex flex-wrap items-center gap-2">
          <CardTitle>{t('usage.title')}</CardTitle>
          <Badge variant="default">{t('usage.count', { count: bindings.length })}</Badge>
        </div>
        <p className="text-sm leading-5 text-muted-foreground">{t('usage.description')}</p>
      </CardHeader>
      <CardContent>
        {sortedBindings.length === 0 ? (
          <div className="rounded-lg border border-dashed px-5 py-10 text-center">
            <MessageCircle aria-hidden="true" className="mx-auto size-7 text-muted-foreground" />
            <p className="mt-3 font-medium">{t('usage.emptyTitle')}</p>
            <p className="mt-1 text-sm text-muted-foreground">{t('usage.emptyDescription')}</p>
          </div>
        ) : (
          <div className="max-h-80 overflow-y-auto overscroll-contain rounded-lg border">
            {sortedBindings.map((binding, index) => {
              const surface = surfaceLabel(binding.target.type);
              const targetName =
                binding.target_name?.trim() || t('usage.unnamedTarget', { surface });
              return (
                <div
                  key={binding.id}
                  className={cn(
                    'grid min-w-0 gap-3 px-3 py-3 sm:grid-cols-[minmax(0,1fr)_auto_auto_auto] sm:items-center',
                    index > 0 && 'border-t'
                  )}
                >
                  <div className="flex min-w-0 items-center gap-3">
                    <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
                      <SurfaceIcon type={binding.target.type} />
                    </div>
                    <p className="truncate text-sm font-medium">{targetName}</p>
                  </div>
                  <Badge className={SURFACE_STYLE[binding.target.type]}>{surface}</Badge>
                  <span
                    className={cn(
                      'inline-flex items-center gap-1.5 text-xs font-medium',
                      binding.status === 'active'
                        ? 'text-success'
                        : binding.status === 'needs_replacement'
                          ? 'text-warning'
                          : 'text-muted-foreground'
                    )}
                  >
                    <span aria-hidden="true" className="size-1.5 rounded-full bg-current" />
                    {statusLabel(binding.status)}
                  </span>
                  <Button
                    asChild
                    size="sm"
                    variant="ghost"
                    className="justify-self-start sm:justify-self-end"
                  >
                    <Link href={connectorBindingTargetHref(binding)}>{t('usage.view')}</Link>
                  </Button>
                </div>
              );
            })}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
