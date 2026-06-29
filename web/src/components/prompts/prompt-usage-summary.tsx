'use client';

import Link from 'next/link';
import { useMemo, useState } from 'react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { WORKFLOW_PERMISSION_ACTIONS } from '@/constants/permissions';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { usePromptUsage } from '@/hooks/prompt/use-prompts';
import { useT } from '@/i18n';
import type { PromptVersion } from '@/services/types/prompt';

interface PromptUsageSummaryProps {
  promptId: string;
  enabled?: boolean;
  versions?: PromptVersion[];
}

function formatElapsedMs(value: number) {
  if (!Number.isFinite(value) || value <= 0) {
    return '0 ms';
  }
  if (value >= 1000) {
    return `${(value / 1000).toFixed(1)} s`;
  }
  return `${Math.round(value)} ms`;
}

function statusVariant(status: string): 'default' | 'destructive' | 'outline' | 'secondary' {
  switch (status) {
    case 'succeeded':
      return 'default';
    case 'failed':
      return 'destructive';
    case 'running':
    case 'paused':
      return 'secondary';
    default:
      return 'outline';
  }
}

function metricCardClassName(isSelected: boolean) {
  return isSelected
    ? 'rounded-lg border border-primary bg-primary/5 p-3'
    : 'rounded-lg border p-3 hover:border-primary/40';
}

function sortText(values: string[]) {
  return [...values].sort((a, b) => a.localeCompare(b));
}

function sortNumber(values: number[]) {
  return [...values].sort((a, b) => a - b);
}

export function PromptUsageSummary({
  promptId,
  enabled = true,
  versions = [],
}: PromptUsageSummaryProps) {
  const t = useT('prompts');
  const { hasAnyPermission } = useAccountPermissions();
  const { usage, isLoading, error } = usePromptUsage(promptId, enabled);
  const [versionFilter, setVersionFilter] = useState<string>('all');
  const [labelFilter, setLabelFilter] = useState<string>('all');
  const isFiltered = versionFilter !== 'all' || labelFilter !== 'all';
  const canOpenWorkflowReference =
    hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.create) ||
    hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.import) ||
    hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.update) ||
    hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.runDraft) ||
    hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.runStop) ||
    hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.debug) ||
    hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.publish) ||
    hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.runtimeConfigManage) ||
    hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.runtimeAccessManage);
  const canOpenWorkflowRunLog = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.logsView);

  const labelToVersion = useMemo(() => {
    const next = new Map<string, number>();
    for (const version of versions) {
      for (const label of version.labels) {
        next.set(label, version.version);
      }
    }
    return next;
  }, [versions]);

  const versionToLabels = useMemo(() => {
    const next = new Map<number, string[]>();
    for (const version of versions) {
      next.set(version.version, version.labels);
    }
    return next;
  }, [versions]);

  const filteredRuns = useMemo(() => {
    const runs = usage?.recent_runs ?? [];
    return runs.filter(run => {
      const matchesVersion =
        versionFilter === 'all' || String(run.prompt_version ?? '') === versionFilter;
      const matchesLabel =
        labelFilter === 'all' || String(run.requested_label ?? '') === labelFilter;
      return matchesVersion && matchesLabel;
    });
  }, [labelFilter, usage?.recent_runs, versionFilter]);

  const filteredReferences = useMemo(() => {
    const references = usage?.references ?? [];
    return references.filter(reference => {
      const effectiveVersion =
        reference.reference_mode === 'version'
          ? reference.version ?? null
          : reference.label
            ? labelToVersion.get(reference.label) ?? null
            : null;
      const effectiveLabels =
        reference.reference_mode === 'version'
          ? versionToLabels.get(reference.version ?? -1) ?? []
          : reference.label
            ? [reference.label]
            : [];

      const matchesVersion =
        versionFilter === 'all' || String(effectiveVersion ?? '') === versionFilter;
      const matchesLabel =
        labelFilter === 'all' || effectiveLabels.includes(labelFilter);
      return matchesVersion && matchesLabel;
    });
  }, [labelFilter, labelToVersion, usage?.references, versionFilter, versionToLabels]);

  const filteredLastRunAt = useMemo(() => {
    if (!filteredRuns.length) return null;
    return filteredRuns
      .map(run => new Date(run.created_at).getTime())
      .sort((a, b) => b - a)[0];
  }, [filteredRuns]);

  const usageInsights = useMemo(() => {
    const references = usage?.references ?? [];
    const recentRuns = usage?.recent_runs ?? [];

    const referenceLabels = sortText(
      Array.from(
        new Set(
          references
            .map(reference => reference.label)
            .filter((label): label is string => Boolean(label))
        )
      )
    );
    const recentLabels = sortText(
      Array.from(
        new Set(
          recentRuns
            .map(run => run.requested_label)
            .filter((label): label is string => Boolean(label))
        )
      )
    );

    const referenceVersions = sortNumber(
      Array.from(
        new Set(
          references
            .map(reference => {
              if (reference.reference_mode === 'version' && reference.version != null) {
                return reference.version;
              }
              if (reference.label && labelToVersion.has(reference.label)) {
                return labelToVersion.get(reference.label) ?? null;
              }
              return null;
            })
            .filter((version): version is number => typeof version === 'number')
        )
      )
    );
    const recentVersions = sortNumber(
      Array.from(
        new Set(
          recentRuns
            .map(run => run.prompt_version)
            .filter((version): version is number => typeof version === 'number')
        )
      )
    );

    return {
      labelsOnlyInRuns: recentLabels.filter(label => !referenceLabels.includes(label)),
      labelsOnlyInReferences: referenceLabels.filter(label => !recentLabels.includes(label)),
      versionsOnlyInRuns: recentVersions.filter(version => !referenceVersions.includes(version)),
      versionsOnlyInReferences: referenceVersions.filter(
        version => !recentVersions.includes(version)
      ),
    };
  }, [labelToVersion, usage?.recent_runs, usage?.references]);

  const hasUsageDrift =
    usageInsights.labelsOnlyInRuns.length > 0 ||
    usageInsights.labelsOnlyInReferences.length > 0 ||
    usageInsights.versionsOnlyInRuns.length > 0 ||
    usageInsights.versionsOnlyInReferences.length > 0;

  if (isLoading) {
    return <div className="rounded-xl border p-4 text-sm text-muted-foreground">{t('usage.loading')}</div>;
  }

  if (error) {
    return (
      <div className="rounded-xl border p-4 text-sm text-muted-foreground">
        {t('usage.loadFailed')}
      </div>
    );
  }

  return (
    <div className="rounded-xl border p-4 space-y-4">
      <div className="space-y-1">
        <h2 className="text-lg font-semibold">{t('usage.title')}</h2>
        <p className="text-sm text-muted-foreground">{t('usage.description')}</p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-4 gap-3">
        <div className="rounded-lg border bg-muted/20 px-4 py-3">
          <div className="text-xs text-muted-foreground">{t('usage.metrics.linkedAgents')}</div>
          <div className="text-2xl font-semibold">{usage?.linked_agents_count ?? 0}</div>
        </div>
        <div className="rounded-lg border bg-muted/20 px-4 py-3">
          <div className="text-xs text-muted-foreground">{t('usage.metrics.linkedNodes')}</div>
          <div className="text-2xl font-semibold">{usage?.linked_nodes_count ?? 0}</div>
        </div>
        <div className="rounded-lg border bg-muted/20 px-4 py-3">
          <div className="text-xs text-muted-foreground">
            {isFiltered ? t('usage.metrics.filteredRuns') : t('usage.metrics.totalRuns')}
          </div>
          <div className="text-2xl font-semibold">
            {isFiltered ? filteredRuns.length : usage?.total_run_count ?? 0}
          </div>
        </div>
        <div className="rounded-lg border bg-muted/20 px-4 py-3">
          <div className="text-xs text-muted-foreground">
            {isFiltered ? t('usage.metrics.filteredLastRunAt') : t('usage.metrics.lastRunAt')}
          </div>
          <div className="text-sm font-medium mt-1">
            {isFiltered
              ? filteredLastRunAt
                ? new Date(filteredLastRunAt).toLocaleString()
                : t('usage.neverRun')
              : usage?.last_run_at
                ? new Date(usage.last_run_at).toLocaleString()
                : t('usage.neverRun')}
          </div>
        </div>
      </div>

      {isFiltered ? (
        <div className="rounded-lg border bg-primary/5 px-4 py-3 text-sm text-muted-foreground">
          {versionFilter !== 'all' ? `${t('usage.filters.version')}: v${versionFilter}` : null}
          {versionFilter !== 'all' && labelFilter !== 'all' ? ' · ' : null}
          {labelFilter !== 'all' ? `${t('usage.filters.label')}: ${labelFilter}` : null}
        </div>
      ) : null}

      {hasUsageDrift ? (
        <div className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 space-y-2">
          <div className="text-sm font-medium text-amber-900">{t('usage.insights.title')}</div>
          <div className="text-sm text-amber-900/80">{t('usage.insights.description')}</div>
          {usageInsights.labelsOnlyInRuns.length > 0 ? (
            <div className="text-sm text-amber-900/80">
              {t('usage.insights.labelsOnlyInRuns')}
              {' '}
              {usageInsights.labelsOnlyInRuns.map(label => (
                <Badge key={`run-label-${label}`} variant="secondary" className="mr-1">
                  {label}
                </Badge>
              ))}
            </div>
          ) : null}
          {usageInsights.labelsOnlyInReferences.length > 0 ? (
            <div className="text-sm text-amber-900/80">
              {t('usage.insights.labelsOnlyInReferences')}
              {' '}
              {usageInsights.labelsOnlyInReferences.map(label => (
                <Badge key={`ref-label-${label}`} variant="secondary" className="mr-1">
                  {label}
                </Badge>
              ))}
            </div>
          ) : null}
          {usageInsights.versionsOnlyInRuns.length > 0 ? (
            <div className="text-sm text-amber-900/80">
              {t('usage.insights.versionsOnlyInRuns')}
              {' '}
              {usageInsights.versionsOnlyInRuns.map(version => (
                <Badge key={`run-version-${version}`} variant="outline" className="mr-1">
                  v{version}
                </Badge>
              ))}
            </div>
          ) : null}
          {usageInsights.versionsOnlyInReferences.length > 0 ? (
            <div className="text-sm text-amber-900/80">
              {t('usage.insights.versionsOnlyInReferences')}
              {' '}
              {usageInsights.versionsOnlyInReferences.map(version => (
                <Badge key={`ref-version-${version}`} variant="outline" className="mr-1">
                  v{version}
                </Badge>
              ))}
            </div>
          ) : null}
        </div>
      ) : null}

      <div className="space-y-3">
        <div className="flex items-center justify-between gap-3">
          <h3 className="text-sm font-semibold">{t('usage.versionMetricsTitle')}</h3>
          {versionFilter !== 'all' ? (
            <Button variant="ghost" size="sm" onClick={() => setVersionFilter('all')}>
              {t('usage.clearFilter')}
            </Button>
          ) : null}
        </div>
        {usage?.version_metrics?.length ? (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {usage.version_metrics.map(metric => (
              <button
                key={metric.version}
                type="button"
                className={metricCardClassName(versionFilter === String(metric.version))}
                onClick={() => setVersionFilter(String(metric.version))}
              >
                <div className="flex items-center justify-between gap-3">
                  <div className="font-medium">v{metric.version}</div>
                  <Badge variant="outline">{metric.run_count}</Badge>
                </div>
                <div className="text-xs text-muted-foreground mt-2">
                  {metric.last_run_at
                    ? `${t('usage.lastActive')}: ${new Date(metric.last_run_at).toLocaleString()}`
                    : t('usage.neverRun')}
                </div>
              </button>
            ))}
          </div>
        ) : (
          <div className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
            {t('usage.emptyVersionMetrics')}
          </div>
        )}
      </div>

      <div className="space-y-3">
        <div className="flex items-center justify-between gap-3">
          <h3 className="text-sm font-semibold">{t('usage.labelMetricsTitle')}</h3>
          {labelFilter !== 'all' ? (
            <Button variant="ghost" size="sm" onClick={() => setLabelFilter('all')}>
              {t('usage.clearFilter')}
            </Button>
          ) : null}
        </div>
        {usage?.label_metrics?.length ? (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {usage.label_metrics.map(metric => (
              <button
                key={metric.label}
                type="button"
                className={metricCardClassName(labelFilter === metric.label)}
                onClick={() => setLabelFilter(metric.label)}
              >
                <div className="flex items-center justify-between gap-3">
                  <div className="font-medium">{metric.label}</div>
                  <Badge variant="outline">{metric.run_count}</Badge>
                </div>
                <div className="text-xs text-muted-foreground mt-2">
                  {metric.last_run_at
                    ? `${t('usage.lastActive')}: ${new Date(metric.last_run_at).toLocaleString()}`
                    : t('usage.neverRun')}
                </div>
              </button>
            ))}
          </div>
        ) : (
          <div className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
            {t('usage.emptyLabelMetrics')}
          </div>
        )}
      </div>

      <div className="space-y-3">
        <h3 className="text-sm font-semibold">{t('usage.referencesTitle')}</h3>
        {filteredReferences.length ? (
          <div className="space-y-3">
            {filteredReferences.map(reference => (
              <div key={`${reference.agent_id}-${reference.node_id}`} className="rounded-lg border p-3">
                {(() => {
                  const referenceMode =
                    reference.reference_mode === 'label'
                      ? 'label'
                      : reference.reference_mode === 'version'
                        ? 'version'
                        : 'managed';
                  return (
                    <>
                <div className="flex items-center justify-between gap-3 flex-wrap">
                  {canOpenWorkflowReference ? (
                    <Link
                      href={`/console/agents/${reference.agent_id}?nodeId=${reference.node_id}`}
                      className="font-medium hover:text-primary"
                    >
                      {reference.agent_name}
                    </Link>
                  ) : (
                    <span className="font-medium text-foreground">{reference.agent_name}</span>
                  )}
                  <div className="text-xs text-muted-foreground">
                    {new Date(reference.updated_at).toLocaleString()}
                  </div>
                </div>
                <div className="text-sm text-muted-foreground mt-1">{reference.node_title || reference.node_id}</div>
                <div className="flex items-center gap-2 flex-wrap mt-2">
                  <Badge variant="outline">{t(`usage.referenceModes.${referenceMode}` as const)}</Badge>
                  {reference.label ? <Badge variant="secondary">{reference.label}</Badge> : null}
                  {reference.version ? <Badge variant="secondary">v{reference.version}</Badge> : null}
                </div>
                    </>
                  );
                })()}
              </div>
            ))}
          </div>
        ) : (
          <div className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
            {usage?.references?.length ? t('usage.emptyFilteredReferences') : t('usage.emptyReferences')}
          </div>
        )}
      </div>

      <div className="space-y-3">
        <h3 className="text-sm font-semibold">{t('usage.recentRunsTitle')}</h3>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          <div className="space-y-2">
            <div className="text-xs text-muted-foreground">{t('usage.filters.version')}</div>
            <Select value={versionFilter} onValueChange={setVersionFilter}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t('usage.filters.allVersions')}</SelectItem>
                {(usage?.version_metrics ?? []).map(metric => (
                  <SelectItem key={metric.version} value={String(metric.version)}>
                    v{metric.version}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <div className="text-xs text-muted-foreground">{t('usage.filters.label')}</div>
            <Select value={labelFilter} onValueChange={setLabelFilter}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t('usage.filters.allLabels')}</SelectItem>
                {(usage?.label_metrics ?? []).map(metric => (
                  <SelectItem key={metric.label} value={metric.label}>
                    {metric.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
        {filteredRuns.length ? (
          <div className="space-y-3">
            {filteredRuns.map(run => (
              <div key={`${run.workflow_run_id || run.node_id}-${run.created_at}`} className="rounded-lg border p-3">
                <div className="flex items-center justify-between gap-3 flex-wrap">
                  {run.workflow_run_id && canOpenWorkflowRunLog ? (
                    <Link
                      href={`/console/agents/${run.agent_id}/logs?runId=${run.workflow_run_id}&tab=execution`}
                      className="font-medium hover:text-primary"
                    >
                      {run.agent_name}
                    </Link>
                  ) : (
                    <div className="font-medium">{run.agent_name}</div>
                  )}
                  <div className="text-xs text-muted-foreground">
                    {new Date(run.created_at).toLocaleString()}
                  </div>
                </div>
                <div className="text-sm text-muted-foreground mt-1">{run.node_title || run.node_id}</div>
                <div className="flex items-center gap-2 flex-wrap mt-2">
                  <Badge variant={statusVariant(run.status)}>{run.status}</Badge>
                  {run.prompt_version ? <Badge variant="outline">v{run.prompt_version}</Badge> : null}
                  {run.requested_label ? <Badge variant="secondary">{run.requested_label}</Badge> : null}
                  {run.requested_version ? <Badge variant="secondary">pin v{run.requested_version}</Badge> : null}
                  <Badge variant="outline">{formatElapsedMs(run.elapsed_time)}</Badge>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
            {usage?.recent_runs?.length ? t('usage.emptyFilteredRuns') : t('usage.emptyRuns')}
          </div>
        )}
      </div>
    </div>
  );
}

export default PromptUsageSummary;
