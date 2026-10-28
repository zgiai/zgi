'use client';

import { useEffect, useMemo, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Activity } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  connectorTargetBindingItems,
  useConnectorTargetBindings,
  usePersonalConnections,
} from '@/hooks/connectors/use-connectors';
import { useDigitalHuman } from '@/hooks/digital-human/use-digital-humans';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import { digitalHumanService } from '@/services/digital-human.service';
import type { DigitalHumanServerWatch, DigitalHumanTask } from '@/services/types/digital-human';
import { getErrorMessage } from '@/utils/error-notifications';
import {
  parseWatchedServices,
  serverWatchProblems,
  SERVER_WATCH_INTERVALS,
  SERVER_WATCH_THRESHOLDS,
} from './digital-human-task-server-watch-model';

const SSH_CONNECTOR_ID = 'serverops_ssh';

/**
 * A task's server watch (ADR 0031): a check without a model that starts this
 * task when a watched service or the server goes down. It appears only for a
 * DigitalHuman with an SSH connection of the reader's, or when a watch exists.
 */
export function TaskServerWatch({ task }: { task: DigitalHumanTask }) {
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const queryKey = ['digital-human', task.digital_human_id, 'task', task.id, 'server-watch'];
  const watchQuery = useQuery({
    queryKey,
    queryFn: () => digitalHumanService.getTaskServerWatch(task.digital_human_id, task.id),
    staleTime: 15_000,
    retry: false,
  });
  const watch = watchQuery.data?.data ?? null;
  const detail = useDigitalHuman(task.digital_human_id);
  const publishedVersionId = detail.data?.data.published?.id;
  const bindingsQuery = useConnectorTargetBindings(
    publishedVersionId
      ? {
          type: 'digital_human',
          id: task.digital_human_id,
          scope: 'published',
          version_id: publishedVersionId,
        }
      : undefined,
    Boolean(publishedVersionId)
  );
  const connectionsQuery = usePersonalConnections({ connector_id: SSH_CONNECTOR_ID });
  const connections = useMemo(() => {
    const names = new Map(
      (connectionsQuery.data?.data?.items ?? []).map(item => [item.id, item.name] as const)
    );
    return connectorTargetBindingItems(bindingsQuery.data?.data)
      .filter(binding => binding.connector_id === SSH_CONNECTOR_ID && binding.status === 'active')
      .map(binding => ({
        id: binding.connection_id,
        name: names.get(binding.connection_id) ?? binding.connection_id.slice(0, 8),
      }));
  }, [bindingsQuery.data, connectionsQuery.data]);

  const [editing, setEditing] = useState(false);
  const [connectionId, setConnectionId] = useState('');
  const [services, setServices] = useState('');
  const [interval, setInterval] = useState(5);
  const [threshold, setThreshold] = useState(2);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setConnectionId(watch?.connection_id ?? connections[0]?.id ?? '');
    setServices(watch?.services.join(', ') ?? '');
    setInterval(watch?.interval_minutes ?? 5);
    setThreshold(watch?.failure_threshold ?? 2);
  }, [watch, connections]);

  if (!watch && connections.length === 0) return null;
  const canChange = task.can_change !== false;
  const ownsWatch = !watch || watch.owned_by_viewer;

  const save = async (enabled: boolean) => {
    const parsed = parseWatchedServices(services);
    if (enabled && (parsed.length === 0 || !connectionId)) {
      toast.error(t('tasks.serverWatch.servicesRequired'));
      return;
    }
    setSaving(true);
    try {
      const response = await digitalHumanService.putTaskServerWatch(
        task.digital_human_id,
        task.id,
        enabled
          ? {
              enabled,
              connection_id: connectionId,
              services: parsed,
              interval_minutes: interval,
              failure_threshold: threshold,
            }
          : { enabled }
      );
      queryClient.setQueryData(queryKey, response);
      setEditing(false);
      toast.success(t(enabled ? 'tasks.serverWatch.saved' : 'tasks.serverWatch.turnedOff'));
    } catch (error) {
      toast.error(getErrorMessage(error) || t('tasks.serverWatch.saveFailed'));
    } finally {
      setSaving(false);
    }
  };

  const problems = watch?.enabled ? serverWatchProblems(watch) : [];
  const form = (
    <div className="mt-2 space-y-2">
      {connections.length > 1 ? (
        <Select value={connectionId} onValueChange={setConnectionId}>
          <SelectTrigger className="h-8 text-xs">
            <SelectValue placeholder={t('tasks.serverWatch.connection')} />
          </SelectTrigger>
          <SelectContent>
            {connections.map(connection => (
              <SelectItem key={connection.id} value={connection.id}>
                {connection.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      ) : null}
      <Input
        className="h-8 text-xs"
        value={services}
        onChange={event => setServices(event.target.value)}
        placeholder={t('tasks.serverWatch.servicesPlaceholder')}
        aria-label={t('tasks.serverWatch.services')}
      />
      <div className="grid grid-cols-2 gap-2">
        <Select value={String(interval)} onValueChange={value => setInterval(Number(value))}>
          <SelectTrigger className="h-8 text-xs" aria-label={t('tasks.serverWatch.interval')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {SERVER_WATCH_INTERVALS.map(minutes => (
              <SelectItem key={minutes} value={String(minutes)}>
                {t('tasks.serverWatch.everyMinutes', { minutes })}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={String(threshold)} onValueChange={value => setThreshold(Number(value))}>
          <SelectTrigger className="h-8 text-xs" aria-label={t('tasks.serverWatch.threshold')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {SERVER_WATCH_THRESHOLDS.map(count => (
              <SelectItem key={count} value={String(count)}>
                {t('tasks.serverWatch.afterFailures', { count })}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="flex gap-2">
        <Button size="sm" className="h-7" disabled={saving} onClick={() => void save(true)}>
          {t('tasks.serverWatch.save')}
        </Button>
        <Button size="sm" variant="ghost" className="h-7" onClick={() => setEditing(false)}>
          {t('common.cancel')}
        </Button>
      </div>
    </div>
  );

  return (
    <div className="rounded-md border px-3 py-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="min-w-0">
          <span className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
            <Activity className="size-3.5" />
            {t('tasks.serverWatch.label')}
          </span>
          <p className="text-[11px] leading-4 text-muted-foreground">
            {watch?.enabled
              ? t('tasks.serverWatch.summary', {
                  services: watch.services.join('、'),
                  minutes: watch.interval_minutes,
                  count: watch.failure_threshold,
                })
              : t('tasks.serverWatch.hint')}
          </p>
        </div>
        {canChange && !editing ? (
          <div className="flex gap-1">
            {ownsWatch ? (
              <Button size="sm" variant="outline" className="h-7" onClick={() => setEditing(true)}>
                {t(watch?.enabled ? 'tasks.serverWatch.edit' : 'tasks.serverWatch.turnOn')}
              </Button>
            ) : null}
            {watch?.enabled ? (
              <Button
                size="sm"
                variant="ghost"
                className="h-7"
                disabled={saving}
                onClick={() => void save(false)}
              >
                {t('tasks.serverWatch.turnOff')}
              </Button>
            ) : null}
          </div>
        ) : null}
      </div>
      {editing ? form : null}
      {watch?.enabled && watch.last_checked_at && !editing ? (
        <p className="mt-1 text-[11px] text-muted-foreground">
          {t('tasks.serverWatch.lastChecked', {
            time: new Date(watch.last_checked_at).toLocaleString(locale),
            outcome: t(outcomeKey(watch)),
          })}
          {problems.length > 0
            ? ` ${t('tasks.serverWatch.problems', {
                items: problems.map(problem => problemLabel(problem, t)).join('、'),
              })}`
            : null}
        </p>
      ) : null}
    </div>
  );
}

const OUTCOMES = [
  'healthy',
  'degraded',
  'triggered',
  'waiting_after_last_run',
  'connection_unusable',
  'check_failed',
  'task_inactive',
  'run_in_progress',
] as const;

type ServerWatchOutcome = (typeof OUTCOMES)[number];

function outcomeKey(
  watch: DigitalHumanServerWatch
): `tasks.serverWatch.outcome.${ServerWatchOutcome}` {
  const outcome = OUTCOMES.find(item => item === watch.last_outcome) ?? 'check_failed';
  return `tasks.serverWatch.outcome.${outcome}`;
}

function problemLabel(
  problem: { name: string; observed: string },
  t: ReturnType<typeof useT<'digitalHumans'>>
) {
  return problem.name === '@host'
    ? t('tasks.serverWatch.hostProblem', { observed: problem.observed })
    : `${problem.name} (${problem.observed})`;
}
