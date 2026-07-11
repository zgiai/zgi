import React from 'react';
import JsonView from '@uiw/react-json-view';
import { lightTheme } from '@uiw/react-json-view/light';
import { formatDate, formatWorkflowElapsedMs } from '@/utils/format';
import { Badge } from '@/components/ui/badge';
import { useT } from '@/i18n';
import { useWorkflowBillingFeedback } from '@/hooks/workflow/use-workflow-billing-feedback';
import type { WorkflowFinishedData } from '../types';

const jsonViewLightTheme = lightTheme as React.CSSProperties;

interface DetailsTabProps {
  runSummary: WorkflowFinishedData | null;
}

// Map run status to badge variant and color dot class
// Return i18n key instead of hardcoded English label for strict localization
type StatusKey = 'running' | 'succeeded' | 'failed' | 'stopped' | 'paused';
const getStatusStyle = (
  status: string
): {
  variant: 'default' | 'secondary' | 'destructive' | 'outline';
  dot: string;
  labelKey?: StatusKey;
  fallback?: string;
} => {
  const s = status.toLowerCase();
  if (s === 'running' || s === 'in_progress') {
    return { variant: 'default', dot: 'bg-blue-500', labelKey: 'running' };
  }
  if (s === 'succeeded' || s === 'success' || s === 'completed' || s === 'partial-succeeded') {
    return { variant: 'secondary', dot: 'bg-green-500', labelKey: 'succeeded' };
  }
  if (s === 'stopped') {
    return { variant: 'outline', dot: 'bg-gray-500', labelKey: 'stopped' };
  }
  if (s === 'paused') {
    return { variant: 'outline', dot: 'bg-warning', labelKey: 'paused' };
  }
  if (s === 'failed' || s === 'error') {
    return { variant: 'destructive', dot: 'bg-red-500', labelKey: 'failed' };
  }
  return { variant: 'secondary', dot: 'bg-gray-400', fallback: status };
};

/**
 * DetailsTab - shows the run summary, inputs and outputs
 */
const DetailsTab: React.FC<DetailsTabProps> = ({ runSummary }) => {
  const t = useT();
  const { getWorkflowRunErrorText } = useWorkflowBillingFeedback('agents');
  if (!runSummary) {
    return (
      <div className="text-sm text-muted-foreground">{t.agents('workflow.noRunDetailsYet')}</div>
    );
  }

  const statusStyle = getStatusStyle(runSummary.status || 'unknown');
  const isPausedStatus = (runSummary.status || '').toLowerCase() === 'paused';

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div className="text-sm font-medium">{t.agents('workflow.runSummary')}</div>
        <Badge variant={statusStyle.variant} className="flex items-center gap-1">
          <span className={`inline-block w-2 h-2 rounded-full ${statusStyle.dot}`} />
          <span className="capitalize">
            {statusStyle.labelKey
              ? t.agents(`workflow.${statusStyle.labelKey}`)
              : (statusStyle.fallback ?? '-')}
          </span>
        </Badge>
      </div>

      <div className="grid grid-cols-2 gap-2 text-xs">
        <div className="p-2 rounded bg-muted">
          <div className="text-muted-foreground">{t.agents('workflow.tokens')}</div>
          <div>{typeof runSummary.total_tokens === 'number' ? runSummary.total_tokens : '-'}</div>
        </div>
        <div className="p-2 rounded bg-muted">
          <div className="text-muted-foreground">{t.agents('workflow.steps')}</div>
          <div>{typeof runSummary.total_steps === 'number' ? runSummary.total_steps : '-'}</div>
        </div>
        <div className="p-2 rounded bg-muted">
          <div className="text-muted-foreground">{t.agents('workflow.startedAt')}</div>
          <div>
            {typeof runSummary.created_at === 'number' ? formatDate(runSummary.created_at) : '-'}
          </div>
        </div>
        <div className="p-2 rounded bg-muted">
          <div className="text-muted-foreground">{t.agents('workflow.finishedAt')}</div>
          <div>
            {typeof runSummary.finished_at === 'number' ? formatDate(runSummary.finished_at) : '-'}
          </div>
        </div>
        {!isPausedStatus ? (
          <div className="p-2 rounded bg-muted col-span-2">
            <div className="text-muted-foreground">{t.agents('workflow.elapsed')}</div>
            <div>{formatWorkflowElapsedMs(runSummary.elapsed_time)}</div>
          </div>
        ) : null}
      </div>

      <div className="rounded-md p-2 bg-muted">
        <div className="text-xs font-medium mb-1">{t.agents('workflow.input')}</div>
        <JsonView
          value={(runSummary.inputs as unknown) ?? {}}
          style={jsonViewLightTheme}
          className="rounded-md overflow-auto max-h-60"
        />
      </div>

      {(() => {
        const s = (runSummary.status || '').toLowerCase();
        const isFailed = s === 'failed' || s === 'error' || s === 'stopped';
        if (isFailed) {
          const rawError = runSummary.error;
          const err =
            rawError && typeof rawError === 'object'
              ? (rawError as Record<string, unknown>)
              : undefined;
          const message =
            getWorkflowRunErrorText(rawError) ??
            (typeof rawError === 'string'
              ? rawError
              : err && typeof err.message === 'string'
                ? (err.message as string)
                : undefined);
          return (
            <div className="rounded-md p-2 bg-red-50 border border-red-200">
              <div className="text-xs font-medium mb-1 text-red-700">
                {t.agents('workflow.error')}
              </div>
              <div className="text-sm text-red-700 whitespace-pre-wrap break-words">
                {message ?? JSON.stringify(err ?? { error: 'Unknown error' })}
              </div>
            </div>
          );
        }
        return (
          <div className="rounded-md p-2 bg-muted">
            <div className="text-xs font-medium mb-1">{t.agents('workflow.output')}</div>
            <JsonView
              value={(runSummary.outputs as unknown) ?? {}}
              style={jsonViewLightTheme}
              className="rounded-md overflow-auto max-h-60"
            />
          </div>
        );
      })()}
    </div>
  );
};

export default DetailsTab;
