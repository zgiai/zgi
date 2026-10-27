'use client';

import { useMemo, useState } from 'react';
import { format, getUnixTime, startOfDay, subDays } from 'date-fns';
import { isAxiosError } from 'axios';
import { RefreshCw, X } from 'lucide-react';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { useOrganizationAuditLogs } from '@/hooks/organization/use-organization-audit-logs';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { useT } from '@/i18n';
import type { DashboardMessages } from '@/i18n/modules/dashboard/en-US';
import type { OrgAuditLog, OrgAuditResult } from '@/services/types/organization';
import { useAuthStore } from '@/store/auth-store';

const PAGE_SIZE = 20;

// Mirrors the actions the API records; unknown actions fall back to their raw value.
const AUDIT_ACTIONS = [
  'member.invite',
  'member.add',
  'member.join',
  'member.join_approve',
  'member.join_reject',
  'member.remove',
  'member.leave',
  'member.status_update',
  'member.role_update',
  'member.password_reset',
  'organization.ownership_transfer',
  'invite_link.update',
  'workspace_member.add',
  'workspace_member.remove',
  'workspace_member.leave',
  'workspace_member.role_update',
  'workspace_member.permissions_update',
  'workspace.ownership_transfer',
  'role.create',
  'role.update',
  'role.permissions_update',
  'role.template_apply',
  'role.delete',
  'role.replace_delete',
  'department.create',
  'department.update',
  'department.delete',
  'department.member_add',
  'department.member_remove',
  'department.member_change',
] as const;

const KNOWN_ACTIONS = new Set<string>(AUDIT_ACTIONS);
const KNOWN_TARGET_TYPES = new Set([
  'member',
  'role',
  'department',
  'workspace',
  'invite_link',
  'join_request',
]);

type DateRangeKey = 'last7Days' | 'last30Days' | 'last90Days' | 'all';
const DATE_RANGE_DAYS: Record<Exclude<DateRangeKey, 'all'>, number> = {
  last7Days: 6,
  last30Days: 29,
  last90Days: 89,
};

const RESULT_VARIANTS: Record<OrgAuditResult, 'success' | 'warning' | 'destructive'> = {
  success: 'success',
  denied: 'warning',
  failed: 'destructive',
};

interface PersonFilter {
  id: string;
  label: string;
}

type AuditMessages = DashboardMessages['organization']['auditLogs'];
type ActionKey = keyof AuditMessages['actions'];
type TargetTypeKey = keyof AuditMessages['targetTypes'];

function actionKey(action: string) {
  return action.replace(/\./g, '_') as ActionKey;
}

function formatValue(value: unknown): string {
  if (value === null || value === undefined || value === '') return '—';
  if (Array.isArray(value)) return value.length === 0 ? '[]' : value.map(formatValue).join(', ');
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

export default function OrganizationAuditLogsPage() {
  const t = useT('dashboard');
  const tCommon = useT('common');
  const { currentOrganization } = useOrganizations(true);
  const user = useAuthStore.use.user();
  const canView = ['owner', 'admin'].includes(
    currentOrganization?.organization_role || user?.organization_role || ''
  );

  const [page, setPage] = useState(1);
  const [action, setAction] = useState('all');
  const [result, setResult] = useState<'all' | OrgAuditResult>('all');
  const [dateRange, setDateRange] = useState<DateRangeKey>('last30Days');
  const [actor, setActor] = useState<PersonFilter | null>(null);
  const [target, setTarget] = useState<PersonFilter | null>(null);
  const [refreshKey, setRefreshKey] = useState(() => Date.now());
  const [selected, setSelected] = useState<OrgAuditLog | null>(null);

  // refreshKey only moves the start of the date range forward; Refresh also
  // refetches, since the query key is unchanged within the same day.
  const queryParams = useMemo(
    () => ({
      page,
      limit: PAGE_SIZE,
      action: action === 'all' ? undefined : action,
      result: result === 'all' ? undefined : result,
      actor_account_id: actor?.id,
      target_account_id: target?.id,
      from:
        dateRange === 'all'
          ? undefined
          : getUnixTime(startOfDay(subDays(new Date(refreshKey), DATE_RANGE_DAYS[dateRange]))),
    }),
    [page, action, result, actor, target, dateRange, refreshKey]
  );
  const query = useOrganizationAuditLogs(currentOrganization?.id, queryParams, canView);
  const data = query.data;
  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const pageStart = total === 0 ? 0 : (page - 1) * PAGE_SIZE + 1;
  const pageEnd = Math.min(page * PAGE_SIZE, total);
  const forbidden = isAxiosError(query.error) && query.error.response?.status === 403;

  const changeFilter = (apply: () => void) => {
    apply();
    setPage(1);
  };
  const hasActiveFilters =
    action !== 'all' || result !== 'all' || dateRange !== 'last30Days' || actor || target;
  const resetFilters = () =>
    changeFilter(() => {
      setAction('all');
      setResult('all');
      setDateRange('last30Days');
      setActor(null);
      setTarget(null);
    });

  const actionLabel = (value: string) =>
    KNOWN_ACTIONS.has(value) ? t(`organization.auditLogs.actions.${actionKey(value)}`) : value;
  const personLabel = (name: string, email: string, id?: string) =>
    name || email || (id ? id.slice(0, 8) : '—');
  const targetLabel = (log: OrgAuditLog) => {
    if (log.target_account_id) {
      return personLabel(log.target_name, log.target_email, log.target_account_id);
    }
    if (!log.target_type) return '—';
    const type = KNOWN_TARGET_TYPES.has(log.target_type)
      ? t(`organization.auditLogs.targetTypes.${log.target_type as TargetTypeKey}`)
      : log.target_type;
    return log.target_id ? `${type} · ${log.target_id.slice(0, 8)}` : type;
  };

  if (!canView) {
    return (
      <div className="p-6">
        <h1 className="text-2xl font-semibold">{t('organization.auditLogs.title')}</h1>
        <p className="mt-4 text-sm text-muted-foreground">{t('organization.auditLogs.noAccess')}</p>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col overflow-auto">
      <div className="space-y-6 p-6">
        <div>
          <h1 className="text-2xl font-semibold">{t('organization.auditLogs.title')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {t('organization.auditLogs.subtitle')}
          </p>
        </div>

        <Card className="border-border/80 shadow-sm">
          <CardContent className="flex flex-col gap-3 p-4 lg:flex-row lg:flex-wrap lg:items-center">
            <Select
              value={dateRange}
              onValueChange={value => changeFilter(() => setDateRange(value as DateRangeKey))}
            >
              <SelectTrigger className="w-full sm:w-[150px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="last7Days">{t('usage.dateRange.last7Days')}</SelectItem>
                <SelectItem value="last30Days">{t('usage.dateRange.last30Days')}</SelectItem>
                <SelectItem value="last90Days">
                  {t('organization.auditLogs.filters.last90Days')}
                </SelectItem>
                <SelectItem value="all">{t('organization.auditLogs.filters.allTime')}</SelectItem>
              </SelectContent>
            </Select>

            <Select value={action} onValueChange={value => changeFilter(() => setAction(value))}>
              <SelectTrigger className="w-full sm:w-[220px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">
                  {t('organization.auditLogs.filters.allActions')}
                </SelectItem>
                {AUDIT_ACTIONS.map(value => (
                  <SelectItem key={value} value={value}>
                    {actionLabel(value)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            <Select
              value={result}
              onValueChange={value =>
                changeFilter(() => setResult(value as 'all' | OrgAuditResult))
              }
            >
              <SelectTrigger className="w-full sm:w-[150px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">
                  {t('organization.auditLogs.filters.allResults')}
                </SelectItem>
                {(['success', 'denied', 'failed'] as const).map(value => (
                  <SelectItem key={value} value={value}>
                    {t(`organization.auditLogs.results.${value}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            {actor ? (
              <Badge variant="outline" className="h-8 gap-1 px-2.5 text-sm font-normal">
                {t('organization.auditLogs.filters.actor', { name: actor.label })}
                <button
                  type="button"
                  aria-label={tCommon('clear')}
                  onClick={() => changeFilter(() => setActor(null))}
                >
                  <X className="h-3.5 w-3.5" />
                </button>
              </Badge>
            ) : null}
            {target ? (
              <Badge variant="outline" className="h-8 gap-1 px-2.5 text-sm font-normal">
                {t('organization.auditLogs.filters.target', { name: target.label })}
                <button
                  type="button"
                  aria-label={tCommon('clear')}
                  onClick={() => changeFilter(() => setTarget(null))}
                >
                  <X className="h-3.5 w-3.5" />
                </button>
              </Badge>
            ) : null}

            <div className="flex gap-2 lg:ml-auto">
              {hasActiveFilters ? (
                <Button variant="ghost" onClick={resetFilters}>
                  {tCommon('resetFilters')}
                </Button>
              ) : null}
              <Button
                variant="outline"
                className="gap-2"
                disabled={query.isFetching}
                onClick={() => {
                  setRefreshKey(Date.now());
                  void query.refetch();
                }}
              >
                <RefreshCw className="h-4 w-4" />
                {tCommon('refresh')}
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card className="border-border/80 shadow-sm">
          <CardContent className="px-0 pb-0 pt-0">
            {query.isError ? (
              <div className="flex flex-col items-center gap-3 px-6 py-12 text-sm text-muted-foreground">
                <p>
                  {forbidden
                    ? t('organization.auditLogs.noAccess')
                    : t('organization.auditLogs.loadFailed')}
                </p>
                {!forbidden ? (
                  <Button variant="outline" size="sm" onClick={() => void query.refetch()}>
                    {tCommon('requestErrors.retry')}
                  </Button>
                ) : null}
              </div>
            ) : (
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="min-w-[156px] pl-6">
                        {t('organization.auditLogs.table.time')}
                      </TableHead>
                      <TableHead>{t('organization.auditLogs.table.actor')}</TableHead>
                      <TableHead>{t('organization.auditLogs.table.action')}</TableHead>
                      <TableHead>{t('organization.auditLogs.table.target')}</TableHead>
                      <TableHead>{t('organization.auditLogs.table.result')}</TableHead>
                      <TableHead className="pr-6 text-right">
                        {t('organization.auditLogs.table.details')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {query.isLoading ? (
                      Array.from({ length: 5 }).map((_, index) => (
                        <TableRow key={index}>
                          <TableCell colSpan={6} className="px-6">
                            <Skeleton className="h-6 w-full" />
                          </TableCell>
                        </TableRow>
                      ))
                    ) : items.length === 0 ? (
                      <TableRow>
                        <TableCell
                          colSpan={6}
                          className="py-12 text-center text-sm text-muted-foreground"
                        >
                          {t('organization.auditLogs.empty')}
                        </TableCell>
                      </TableRow>
                    ) : (
                      items.map(log => (
                        <TableRow key={log.id}>
                          <TableCell className="whitespace-nowrap pl-6 text-sm text-muted-foreground">
                            {format(new Date(log.created_at), 'yyyy-MM-dd HH:mm:ss')}
                          </TableCell>
                          <TableCell className="max-w-[200px]">
                            {log.actor_account_id ? (
                              <button
                                type="button"
                                className="truncate text-left text-sm hover:underline"
                                title={log.actor_email}
                                onClick={() =>
                                  changeFilter(() =>
                                    setActor({
                                      id: log.actor_account_id as string,
                                      label: personLabel(
                                        log.actor_name,
                                        log.actor_email,
                                        log.actor_account_id
                                      ),
                                    })
                                  )
                                }
                              >
                                {personLabel(log.actor_name, log.actor_email, log.actor_account_id)}
                              </button>
                            ) : (
                              '—'
                            )}
                          </TableCell>
                          <TableCell className="whitespace-nowrap text-sm">
                            {actionLabel(log.action)}
                          </TableCell>
                          <TableCell className="max-w-[220px]">
                            {log.target_account_id ? (
                              <button
                                type="button"
                                className="truncate text-left text-sm hover:underline"
                                title={log.target_email}
                                onClick={() =>
                                  changeFilter(() =>
                                    setTarget({
                                      id: log.target_account_id as string,
                                      label: targetLabel(log),
                                    })
                                  )
                                }
                              >
                                {targetLabel(log)}
                              </button>
                            ) : (
                              <span className="truncate text-sm text-muted-foreground">
                                {targetLabel(log)}
                              </span>
                            )}
                          </TableCell>
                          <TableCell>
                            <Badge variant={RESULT_VARIANTS[log.result] ?? 'subtle'}>
                              {t(`organization.auditLogs.results.${log.result}`)}
                            </Badge>
                          </TableCell>
                          <TableCell className="pr-6 text-right">
                            <Button variant="ghost" size="sm" onClick={() => setSelected(log)}>
                              {t('organization.auditLogs.table.view')}
                            </Button>
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </div>
            )}

            <div className="flex flex-col gap-3 border-t px-6 py-4 sm:flex-row sm:items-center sm:justify-between">
              <span className="text-sm text-muted-foreground">
                {t('organization.auditLogs.pagination.summary', {
                  start: pageStart,
                  end: pageEnd,
                  total,
                })}
              </span>
              <div className="flex gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => setPage(current => Math.max(1, current - 1))}
                  disabled={page === 1 || query.isFetching}
                >
                  {t('organization.auditLogs.pagination.previous')}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => setPage(current => current + 1)}
                  disabled={!data?.has_more || query.isFetching}
                >
                  {t('organization.auditLogs.pagination.next')}
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      <Sheet
        open={selected !== null}
        onOpenChange={open => (!open ? setSelected(null) : undefined)}
      >
        <SheetContent className="w-full overflow-y-auto sm:max-w-lg">
          {selected ? (
            <>
              <SheetHeader>
                <SheetTitle>{actionLabel(selected.action)}</SheetTitle>
                <SheetDescription>
                  {format(new Date(selected.created_at), 'yyyy-MM-dd HH:mm:ss')}
                </SheetDescription>
              </SheetHeader>
              <AuditLogDetail
                log={selected}
                actorLabel={personLabel(
                  selected.actor_name,
                  selected.actor_email,
                  selected.actor_account_id
                )}
                targetLabel={targetLabel(selected)}
              />
            </>
          ) : null}
        </SheetContent>
      </Sheet>
    </div>
  );
}

function AuditLogDetail({
  log,
  actorLabel,
  targetLabel,
}: {
  log: OrgAuditLog;
  actorLabel: string;
  targetLabel: string;
}) {
  const t = useT('dashboard');
  const fields = Array.from(
    new Set([...Object.keys(log.before ?? {}), ...Object.keys(log.after ?? {})])
  ).sort();
  const meta: Array<[string, string]> = [
    [t('organization.auditLogs.table.actor'), actorLabel],
    [t('organization.auditLogs.table.target'), targetLabel],
    [t('organization.auditLogs.table.result'), t(`organization.auditLogs.results.${log.result}`)],
    [t('organization.auditLogs.detail.workspace'), log.workspace_id || '—'],
    [t('organization.auditLogs.detail.ip'), log.ip || '—'],
    [t('organization.auditLogs.detail.requestId'), log.request_id || '—'],
  ];

  return (
    <div className="mt-6 space-y-6 px-4 pb-6 text-sm">
      <dl className="grid grid-cols-[120px_1fr] gap-x-3 gap-y-2">
        {meta.map(([label, value]) => (
          <div key={label} className="contents">
            <dt className="text-muted-foreground">{label}</dt>
            <dd className="break-all">{value}</dd>
          </div>
        ))}
      </dl>

      <div>
        <h3 className="mb-2 font-medium">{t('organization.auditLogs.detail.changes')}</h3>
        {fields.length === 0 ? (
          <p className="text-muted-foreground">{t('organization.auditLogs.detail.noChanges')}</p>
        ) : (
          <div className="overflow-hidden rounded-md border">
            <table className="w-full table-fixed text-left">
              <thead className="bg-muted/40 text-xs text-muted-foreground">
                <tr>
                  <th className="w-[30%] px-3 py-2 font-medium">
                    {t('organization.auditLogs.detail.field')}
                  </th>
                  <th className="w-[35%] px-3 py-2 font-medium">
                    {t('organization.auditLogs.detail.before')}
                  </th>
                  <th className="px-3 py-2 font-medium">
                    {t('organization.auditLogs.detail.after')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {fields.map(field => (
                  <tr key={field} className="border-t align-top">
                    <td className="break-all px-3 py-2 font-mono text-xs">{field}</td>
                    <td className="break-all px-3 py-2">{formatValue(log.before?.[field])}</td>
                    <td className="break-all px-3 py-2">{formatValue(log.after?.[field])}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
