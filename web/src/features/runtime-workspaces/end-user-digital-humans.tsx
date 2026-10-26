'use client';

import * as React from 'react';
import { useQuery } from '@tanstack/react-query';
import { Bot } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { RUNTIME_WORKSPACE_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import { runtimeWorkspaceService } from '@/services/runtime-workspace.service';
import type { RuntimeEndUserDigitalHuman } from '@/services/types/runtime-workspace';

function formatTime(value?: string | null) {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(
    date
  );
}

const connectionStates = ['connected', 'needs_connection', 'reconnect_required'] as const;

/**
 * @component EndUserDigitalHumans
 * @category Feature
 * @status Beta
 * @description What each DigitalHuman of an external access keeps for one end
 * user (ADR 0006): their instance, its memory, their tasks, and the
 * connections they supplied. Read only; erasing the end user removes it all.
 * @usage Rendered as the DigitalHumans tab of the end-user detail.
 * @example
 * <EndUserDigitalHumans runtimeWorkspaceId={id} subjectId={user.subject_id} />
 */
export function EndUserDigitalHumans({
  runtimeWorkspaceId,
  subjectId,
}: {
  runtimeWorkspaceId: string;
  subjectId: string;
}) {
  const t = useT('workspace');
  const panels = useQuery({
    queryKey: RUNTIME_WORKSPACE_KEYS.endUserDigitalHumans(runtimeWorkspaceId, subjectId),
    queryFn: () => runtimeWorkspaceService.listEndUserDigitalHumans(runtimeWorkspaceId, subjectId),
    enabled: Boolean(runtimeWorkspaceId && subjectId),
    retry: false,
  });

  if (panels.isLoading) {
    return (
      <div className="space-y-2">
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-24 w-full" />
      </div>
    );
  }
  const items = panels.data ?? [];
  if (items.length === 0) {
    return <p className="text-xs text-muted-foreground">{t('runtime.endUserDH.empty')}</p>;
  }
  return (
    <div className="space-y-4">
      {items.map(panel => (
        <DigitalHumanPanel key={panel.digital_human_id} panel={panel} />
      ))}
    </div>
  );
}

function DigitalHumanPanel({ panel }: { panel: RuntimeEndUserDigitalHuman }) {
  const t = useT('workspace');
  return (
    <section className="space-y-4 rounded-lg border p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <Bot className="size-4 shrink-0 text-muted-foreground" />
          <div className="min-w-0">
            <div className="truncate text-sm font-medium">{panel.name || panel.app_key}</div>
            <code className="text-[11px] text-muted-foreground">{panel.app_key}</code>
          </div>
        </div>
        {panel.instance ? (
          <Badge variant={panel.instance.status === 'active' ? 'success' : 'warning'}>
            {t(
              `runtime.endUserDH.instanceStatus.${panel.instance.status as 'active' | 'suspended' | 'erased'}`
            )}
          </Badge>
        ) : (
          <Badge variant="outline">{t('runtime.endUserDH.noInstance')}</Badge>
        )}
      </div>
      {panel.instance ? (
        <dl className="grid grid-cols-2 gap-2 text-xs">
          <div>
            <dt className="text-muted-foreground">{t('runtime.endUserDH.firstSeen')}</dt>
            <dd>{formatTime(panel.instance.first_seen_at)}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">{t('runtime.endUserDH.lastActive')}</dt>
            <dd>{formatTime(panel.instance.last_active_at)}</dd>
          </div>
        </dl>
      ) : null}

      <div className="space-y-2">
        <h4 className="text-xs font-medium">
          {t('runtime.endUserDH.memory', { count: panel.memory_count })}
        </h4>
        {panel.memory.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t('runtime.endUserDH.noMemory')}</p>
        ) : (
          <ul className="space-y-1.5">
            {panel.memory.map(memory => (
              <li key={memory.id} className="rounded-md bg-muted/40 px-3 py-2 text-xs">
                <div className="flex items-center justify-between gap-2 text-muted-foreground">
                  <code className="truncate">{memory.memory_key}</code>
                  <span className="shrink-0">{formatTime(memory.updated_at)}</span>
                </div>
                <p className="mt-1 whitespace-pre-wrap break-words">{memory.content}</p>
              </li>
            ))}
          </ul>
        )}
        {panel.memory_count > panel.memory.length ? (
          <p className="text-xs text-muted-foreground">
            {t('runtime.endUserDH.moreMemory', { count: panel.memory.length })}
          </p>
        ) : null}
      </div>

      <div className="space-y-2">
        <h4 className="text-xs font-medium">
          {t('runtime.endUserDH.tasks', { count: panel.task_count })}
        </h4>
        {panel.tasks.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t('runtime.endUserDH.noTasks')}</p>
        ) : (
          <ul className="divide-y rounded-md border text-xs">
            {panel.tasks.map(task => (
              <li key={task.id} className="flex items-center justify-between gap-2 px-3 py-2">
                <span className="truncate">{task.name}</span>
                <span className="flex shrink-0 items-center gap-2 text-muted-foreground">
                  <Badge variant="subtle" className="font-normal">
                    {task.work_status || task.status}
                  </Badge>
                  {formatTime(task.last_run_at || task.created_at)}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>

      {panel.connections.length > 0 ? (
        <div className="space-y-2">
          <h4 className="text-xs font-medium">{t('runtime.endUserDH.connections')}</h4>
          <ul className="space-y-1 text-xs">
            {panel.connections.map(slot => (
              <li key={slot.slot_key} className="flex items-center justify-between gap-2">
                <span className="truncate">{slot.name}</span>
                <Badge
                  variant={slot.state === 'connected' ? 'success' : 'outline'}
                  className="font-normal"
                >
                  {(connectionStates as readonly string[]).includes(slot.state)
                    ? t(
                        `runtime.endUserDH.connectionStates.${slot.state as (typeof connectionStates)[number]}`
                      )
                    : slot.state}
                </Badge>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </section>
  );
}
