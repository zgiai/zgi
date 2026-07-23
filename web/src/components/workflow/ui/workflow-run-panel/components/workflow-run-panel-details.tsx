import React from 'react';
import JsonView from '@uiw/react-json-view';
import { lightTheme } from '@uiw/react-json-view/light';
import { formatDate, formatWorkflowElapsedMs } from '@/utils/format';
import { useT } from '@/i18n';
import { useWorkflowBillingFeedback } from '@/hooks/workflow/use-workflow-billing-feedback';
import { RunStatusBadge } from '../../run-status-badge';
import type { WorkflowFinishedData } from '../types';

const jsonViewLightTheme = lightTheme as React.CSSProperties;

interface DetailsTabProps {
  runSummary: WorkflowFinishedData | null;
}

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

  const isPausedStatus = (runSummary.status || '').toLowerCase() === 'paused';

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div className="text-sm font-medium">{t.agents('workflow.runSummary')}</div>
        <RunStatusBadge status={runSummary.status} />
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
        const isFailed = s === 'failed' || s === 'error';
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
