'use client';

import { useState, type ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import { AlertTriangle, ArrowRight, CheckCircle2, ChevronDown, CircleDot, Pencil } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import { organizationService } from '@/services/organization.service';
import { useOrganizationStore } from '@/store/organization-store';
import type { DigitalHumanTaskActivity, DigitalHumanTaskPage, DigitalHumanTaskRun } from '@/services/types/digital-human';

type TaskTranslator = ReturnType<typeof useT<'digitalHumans'>>;

// showResult is off when the task's reports already carry what the run produced.
export function DigitalHumanTaskRunSummary({ run, showResult = true, children }: { run: DigitalHumanTaskRun; showResult?: boolean; children?: ReactNode }) {
  const t = useT('digitalHumans');
  return (
    <div className="rounded-md border bg-muted/20 p-3 text-xs">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="font-medium">{t('tasks.detail.attempt', { value: run.attempt })}</span>
        <Badge variant={run.status === 'succeeded' ? 'success' : run.status === 'failed' ? 'destructive' : 'subtle'}>
          {runStatusLabel(run.status, t)}
        </Badge>
      </div>
      <p className="mt-1 text-muted-foreground">{formatTaskDate(run.finished_at ?? run.created_at)}</p>
      {run.error_summary ? <p className="mt-1 whitespace-pre-wrap text-destructive">{run.error_summary}</p> : null}
      {run.result_delivery_status ? (
        <p className="mt-1 text-muted-foreground">
          {t('tasks.detail.resultDelivery')}：{runDeliveryStatusLabel(run.result_delivery_status, t)}
          {run.result_delivery_error ? ` · ${run.result_delivery_error}` : ''}
        </p>
      ) : null}
      {showResult && run.result_summary ? <p className="mt-1 whitespace-pre-wrap text-foreground">{run.result_summary}</p> : null}
      {children}
    </div>
  );
}

export function DigitalHumanTaskActivityItem({
  activity, locale, isLast, defaultOpen = false,
}: {
  activity: DigitalHumanTaskActivity;
  locale?: string;
  isLast: boolean;
  defaultOpen?: boolean;
}) {
  const t = useT('digitalHumans');
  const memberId = activity.actor_type === 'account' &&
    /^[\da-f]{8}-[\da-f]{4}-[\da-f]{4}-[\da-f]{4}-[\da-f]{12}$/i.test(activity.actor_ref)
    ? activity.actor_ref : null;
  const organizationId = useOrganizationStore.use.currentOrganization()?.id;
  const { data: member } = useQuery({
    queryKey: ['digital-human-task-actor', organizationId, memberId],
    queryFn: () => organizationService.getCurrentOrganizationMember(memberId ?? ''),
    enabled: Boolean(memberId && organizationId),
    staleTime: 2 * 60 * 1000,
    retry: false,
  });
  const actor = member?.member_name?.trim() || member?.name?.trim() || activityActorLabel(activity.actor_type, t);
  return (
    <li className="relative list-none pl-6">
      {!isLast ? <span aria-hidden="true" className="absolute bottom-0 left-[7px] top-4 w-px bg-border" /> : null}
      <span aria-hidden="true" className={`absolute left-0 top-0.5 flex size-4 items-center justify-center rounded-full bg-background ${activityTone(activity.action)}`}>
        <ActivityIcon action={activity.action} />
      </span>
      <div className="pb-4 last:pb-0">
        <div className="flex flex-wrap items-baseline justify-between gap-x-2 gap-y-0.5">
          <span className="text-xs font-medium text-foreground">{activityLabel(activity.action, t)}</span>
          <time dateTime={activity.created_at} title={formatTaskDate(activity.created_at)} className="text-[11px] text-muted-foreground">
            {formatActivityTime(activity.created_at, locale)}
          </time>
        </div>
        <div className="mt-0.5 text-[11px] text-muted-foreground">{actor}</div>
        <ActivityChangeDetails activity={activity} locale={locale} t={t} defaultOpen={defaultOpen} />
      </div>
    </li>
  );
}

export function DigitalHumanTaskHistoryPager({
  data, page, onPage,
}: {
  data: DigitalHumanTaskPage<unknown> | null;
  page: number;
  onPage: (page: number) => void;
}) {
  const t = useT('digitalHumans');
  if (!data || (data.total === 0 && page === 1)) return null;
  const start = data.items.length ? (page - 1) * data.page_size + 1 : 0;
  const end = data.items.length ? start + data.items.length - 1 : 0;
  return (
    <div className="mt-3 flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
      <span>{t('tasks.pagination.range', { from: start, to: end, total: data.total })}</span>
      <div className="flex gap-2">
        <Button variant="outline" size="sm" disabled={page <= 1}
          onClick={() => onPage(page - 1)}>{t('tasks.pagination.previous')}</Button>
        <Button variant="outline" size="sm" disabled={!data.has_more}
          onClick={() => onPage(page + 1)}>{t('tasks.pagination.next')}</Button>
      </div>
    </div>
  );
}

function activityLabel(action: string, t: TaskTranslator): string {
  const labels: Record<string, string> = {
    created: t('tasks.activity.created'), updated: t('tasks.activity.updated'),
    status_changed: t('tasks.activity.status_changed'), confirm: t('tasks.activity.confirm'),
    decline: t('tasks.activity.decline'), run_now: t('tasks.activity.run_now'),
    pause: t('tasks.activity.pause'), resume: t('tasks.activity.resume'),
    retry: t('tasks.activity.retry'), cancel: t('tasks.activity.cancel'),
    archive: t('tasks.activity.archive'), blocked: t('tasks.activity.blocked'),
    run_finished: t('tasks.activity.run_finished'), proposal_expired: t('tasks.activity.proposal_expired'),
    status_repaired: t('tasks.activity.status_repaired'),
    report_accepted: t('tasks.activity.report_accepted'), rework_requested: t('tasks.activity.rework_requested'),
    report_improved: t('tasks.activity.report_improved'),
    result_delivered: t('tasks.activity.result_delivered'), result_delivery_failed: t('tasks.activity.result_delivery_failed'),
  };
  return labels[action] || action.replace(/[_-]+/g, ' ').replace(/\b\w/g, character => character.toUpperCase());
}

function activityActorLabel(actorType: string, t: TaskTranslator): string {
  if (actorType === 'system') return t('tasks.activity.system');
  if (actorType === 'digital_human') return t('tasks.activity.actorDigitalHuman');
  if (actorType === 'external') return t('tasks.activity.actorExternal');
  return t('tasks.activity.actorAccount');
}

function activityTone(action: string): string {
  if (action === 'created' || action === 'confirm' || action === 'run_finished') return 'text-emerald-600';
  if (action === 'decline' || action === 'cancel' || action === 'archive' || action === 'blocked') return 'text-destructive';
  return 'text-primary';
}

function ActivityIcon({ action }: { action: string }) {
  if (action === 'confirm' || action === 'run_finished') return <CheckCircle2 className="size-2.5" />;
  if (action === 'blocked') return <AlertTriangle className="size-2.5" />;
  if (action === 'updated' || action === 'status_changed') return <Pencil className="size-2.5" />;
  return <CircleDot className="size-2.5" />;
}

function ActivityChangeDetails({ activity, locale, t, defaultOpen }: {
  activity: DigitalHumanTaskActivity;
  locale?: string;
  t: TaskTranslator;
  defaultOpen: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const changes = formatActivityChanges(activity.changes, activity.action, locale);
  if (changes.length === 0) return null;
  return (
    <div className="mt-1.5">
      <button type="button"
        className="inline-flex min-h-7 items-center gap-1 rounded-md px-1.5 text-[11px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        aria-expanded={open} onClick={() => setOpen(value => !value)}>
        <ChevronDown className={`size-3 transition-transform ${open ? '' : '-rotate-90'}`} />
        {open ? t('tasks.activity.hideChanges') : t('tasks.activity.viewChanges')}
      </button>
      {open ? (
        <div className="mt-1.5 space-y-1 rounded-md bg-muted/50 px-2.5 py-2 text-[11px] leading-5">
          {changes.map(change => (
            <div key={change.key} className="grid gap-0.5 sm:grid-cols-[minmax(5rem,8rem)_minmax(0,1fr)] sm:gap-2">
              <span className="text-muted-foreground">{change.label}</span>
              <span className="min-w-0 break-words font-medium text-foreground">
                {change.before !== undefined ? (
                  <>
                    <span className="text-muted-foreground line-through">{formatActivityValue(change.key, change.before, locale)}</span>
                    <ArrowRight className="mx-1 inline-block size-3 align-[-2px] text-muted-foreground" />
                  </>
                ) : null}
                {change.after !== undefined ? formatActivityValue(change.key, change.after, locale) : t('tasks.activity.removed')}
              </span>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function formatActivityTime(value: string, locale?: string): string {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return value;
  const elapsed = Math.max(0, Date.now() - timestamp);
  const minute = 60 * 1000;
  const hour = 60 * minute;
  const day = 24 * hour;
  if (elapsed < minute) return locale?.startsWith('zh') ? '刚刚' : 'Just now';
  if (elapsed < hour) {
    const count = Math.max(1, Math.floor(elapsed / minute));
    return locale?.startsWith('zh') ? `${count} 分钟前` : `${count}m ago`;
  }
  if (elapsed < day) {
    const count = Math.max(1, Math.floor(elapsed / hour));
    return locale?.startsWith('zh') ? `${count} 小时前` : `${count}h ago`;
  }
  if (elapsed < 7 * day) {
    const count = Math.max(1, Math.floor(elapsed / day));
    return locale?.startsWith('zh') ? `${count} 天前` : `${count}d ago`;
  }
  return new Intl.DateTimeFormat(locale === 'zh-Hans' ? 'zh-CN' : locale, {
    month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit',
  }).format(new Date(timestamp));
}

function formatActivityChanges(changes: Record<string, unknown> | undefined, action: string, locale?: string):
  Array<{ key: string; label: string; before?: unknown; after?: unknown }> {
  if (!changes) return [];
  const labels: Record<string, string> = locale?.startsWith('zh')
    ? { name: '名称', goal: '目标', priority: '优先级', work_status: '状态', lifecycle_status: '生命周期',
        delivery_mode: '交付方式', schedule: '执行计划', reason: '原因', run_status: '执行状态', digital_human_status: '数字人状态',
        visibility: '可见范围', success_criteria: '成功标准' }
    : { name: 'Name', goal: 'Goal', priority: 'Priority', work_status: 'Status', lifecycle_status: 'Lifecycle',
        delivery_mode: 'Delivery', schedule: 'Schedule', reason: 'Reason', run_status: 'Run status', digital_human_status: 'Digital human status',
        visibility: 'Visibility', success_criteria: 'Success criteria' };
  const entries: Array<{ key: string; label: string; before?: unknown; after?: unknown }> = [];
  for (const [key, rawValue] of Object.entries(changes)) {
    if (rawValue === undefined || rawValue === null) continue;
    if (isActivityChange(rawValue)) {
      entries.push({ key, label: labels[key] ?? key, before: rawValue.before, after: rawValue.after });
    } else if (typeof rawValue === 'object') {
      entries.push({ key, label: labels[key] ?? key, after: activityObjectSummary(key, locale) });
    } else {
      entries.push({ key, label: labels[key] ?? key, before: inferredActivityBefore(action, key, rawValue), after: rawValue });
    }
  }
  return entries;
}

function isActivityChange(value: unknown): value is { before?: unknown; after?: unknown } {
  return typeof value === 'object' && value !== null && ('before' in value || 'after' in value);
}

function inferredActivityBefore(action: string, key: string, after: unknown): unknown {
  if (action !== 'confirm') return undefined;
  if (key === 'work_status' && after === 'pending') return 'proposed';
  if (key === 'status' && after === 'active') return 'draft';
  return undefined;
}

function activityObjectSummary(key: string, locale?: string): string {
  if (key === 'schedule') return locale?.startsWith('zh') ? '执行计划已更新' : 'Execution plan updated';
  return locale?.startsWith('zh') ? '已更新' : 'Updated';
}

function formatActivityValue(key: string, value: unknown, locale?: string): string {
  const zh = locale?.startsWith('zh');
  const maps: Record<string, Record<string, string>> = {
    work_status: zh
      ? { proposed: '待确认', pending: '待执行', in_progress: '执行中', waiting: '等待输入', blocked: '执行受阻', completed: '已完成', canceled: '已取消' }
      : { proposed: 'Needs confirmation', pending: 'Ready to run', in_progress: 'In progress', waiting: 'Waiting for input', blocked: 'Blocked', completed: 'Completed', canceled: 'Canceled' },
    status: zh
      ? { draft: '未启用', active: '已启用', paused: '已暂停', archived: '已归档' }
      : { draft: 'Not active', active: 'Enabled', paused: 'Paused', archived: 'Archived' },
    visibility: zh
      ? { requester: '仅发起人', conversation: '仅本会话', workspace: '工作空间', restricted: '受限' }
      : { requester: 'Requester only', conversation: 'Conversation only', workspace: 'Workspace', restricted: 'Restricted' },
    priority: zh
      ? { low: '低', normal: '普通', high: '高', urgent: '紧急' }
      : { low: 'Low', normal: 'Normal', high: 'High', urgent: 'Urgent' },
    delivery_mode: zh
      ? { task_only: '仅保留在待办详情', source_conversation: '回复到来源会话' }
      : { task_only: 'Keep in task details', source_conversation: 'Reply to source conversation' },
    reason: zh ? { execution_unavailable: '执行环境不可用' } : { execution_unavailable: 'Execution environment unavailable' },
    run_status: zh
      ? { queued: '排队中', running: '执行中', succeeded: '已完成', failed: '失败', canceled: '已取消' }
      : { queued: 'Queued', running: 'Running', succeeded: 'Succeeded', failed: 'Failed', canceled: 'Canceled' },
  };
  if (typeof value === 'string') return maps[key]?.[value] ?? value;
  if (typeof value === 'boolean') return value ? (zh ? '是' : 'Yes') : zh ? '否' : 'No';
  if (Array.isArray(value)) return value.join(zh ? '、' : ', ');
  if (value === null || value === undefined) return zh ? '无' : 'None';
  return String(value);
}

function formatTaskDate(value: string): string {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return value;
  return new Date(timestamp).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
}

function runStatusLabel(status: string, t: TaskTranslator) {
  switch (status) {
    case 'queued': return t('tasks.runStatus.queued');
    case 'running': return t('tasks.runStatus.running');
    case 'succeeded': return t('tasks.runStatus.succeeded');
    case 'failed': return t('tasks.runStatus.failed');
    case 'canceled': return t('tasks.runStatus.canceled');
    default: return t('tasks.runStatus.unknown');
  }
}

function runDeliveryStatusLabel(status: NonNullable<DigitalHumanTaskRun['result_delivery_status']>, t: TaskTranslator) {
  const key = ['pending', 'processing', 'succeeded', 'failed', 'unknown', 'dismissed'].includes(status) ? status : 'unknown';
  return t(`tasks.deliveryStatus.${key}` as const);
}
