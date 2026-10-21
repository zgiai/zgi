'use client';

import { useCallback, useMemo, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Ban, Loader2 } from 'lucide-react';
import { AgentRuntimeLogDetailDrawer } from '@/app/console/agents/[agentId]/logs/_components/agent-runtime-log-detail-drawer';
import { Button } from '@/components/ui/button';
import { ApprovalRuntimeForm } from '@/components/workflow/approval/approval-runtime-form';
import { normalizeApprovalRuntimeForm } from '@/components/workflow/approval/runtime-events';
import { DigitalHumanRuntimeTransport } from '@/components/chat/transports/digital-human-runtime-transport';
import type { AIChatStreamCallbacks } from '@/components/chat/transports/aichat-transport';
import {
  ConnectorRuntimeConfirmationCard,
  type ConnectorRuntimeDecisionHandler,
} from '@/components/connectors/runtime/connector-runtime-confirmation-card';
import { submitConnectorRuntimeDecision } from '@/components/connectors/runtime/connector-runtime-transport';
import { parseDigitalHumanRunConnectorConfirmation } from '@/components/digital-human/digital-human-run-confirmation-presentation';
import { DigitalHumanBackgroundRunFiles } from './digital-human-background-run-files';
import {
  digitalHumanRunApprovalReceipt,
  isActiveDigitalHumanRunStatus,
  digitalHumanRunStartedByPerson,
} from '@/components/digital-human/digital-human-run-presentation';
import {
  useDigitalHumanRun,
  useDigitalHumanRunRuntimeLog,
} from '@/hooks/digital-human/use-digital-humans';
import { DIGITAL_HUMAN_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import type { AgentRuntimeRunItem } from '@/services/types/agent-runtime-log';
import type { DigitalHumanRun } from '@/services/types/digital-human';

const discardWorkflowApprovalStream: AIChatStreamCallbacks = {
  onMessageStart: () => undefined,
  onAgentProgress: () => undefined,
  onIntermediateAnswer: () => undefined,
  onUserInputRequested: () => undefined,
  onFileParseStart: () => undefined,
  onFileParseEnd: () => undefined,
  onFileParseError: () => undefined,
  onSkillLoadStart: () => undefined,
  onSkillLoadEnd: () => undefined,
  onSkillReferenceRead: () => undefined,
  onSkillCallStart: () => undefined,
  onSkillCallEnd: () => undefined,
  onSkillCallError: () => undefined,
  onSkillArtifactCreated: () => undefined,
  onMemoryMutation: () => undefined,
  onMessageChunk: () => undefined,
  onMessageRetract: () => undefined,
  onMessageEnd: () => undefined,
  onErrorEvent: () => undefined,
  onRequestError: () => undefined,
  onClose: () => undefined,
};

interface DigitalHumanRunDetailDrawerProps {
  digitalHumanId: string;
  runId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCancel: (runId: string) => Promise<void>;
  cancelPending: boolean;
}

export function DigitalHumanRunDetailDrawer({
  digitalHumanId,
  runId,
  open,
  onOpenChange,
  onCancel,
  cancelPending,
}: DigitalHumanRunDetailDrawerProps) {
  const t = useT('digitalHumans');
  const queryClient = useQueryClient();
  const runQuery = useDigitalHumanRun(digitalHumanId, runId, open);
  const run = runQuery.data?.data ?? null;
  const active = run ? isActiveDigitalHumanRunStatus(run.status) : false;
  const runtimeQuery = useDigitalHumanRunRuntimeLog(digitalHumanId, runId, open, !run || active);
  const runtimeLog = runtimeQuery.data?.data;
  const runtimeSteps = useMemo(
    () =>
      (runtimeLog?.steps ?? []).map(step => {
        if (step.type !== 'channel_context') return step;
        const output = isRecord(step.output) ? step.output : null;
        return {
          ...step,
          title:
            output?.error_code === 'channel_context_not_supported'
              ? t('runs.channelContext.notSupported')
              : t('runs.channelContext.title'),
        };
      }),
    [runtimeLog?.steps, t]
  );
  const selectedRun = useMemo(() => (run ? toRuntimeRunItem(run) : null), [run]);
  const error =
    runQuery.isError || runtimeQuery.isError ? t('runs.detailErrorDescription') : undefined;

  // Only a durable `waiting_approval` run with a matching, strictly parsed
  // projection can offer approval. Everything else keeps the existing run
  // cancellation path and never renders consent UI.
  const approvalWait = Boolean(
    run && run.status === 'waiting' && run.waiting_reason === 'waiting_approval'
  );
  const approvalReceipt = run ? digitalHumanRunApprovalReceipt(run) : null;
  const [workflowApprovalPending, setWorkflowApprovalPending] = useState(false);
  const workflowApproval = runtimeLog?.pending_workflow_approval;
  const workflowApprovalForm = useMemo(
    () => normalizeApprovalRuntimeForm(workflowApproval?.approval_form),
    [workflowApproval?.approval_form]
  );
  const connectorConfirmation = useMemo(
    () =>
      parseDigitalHumanRunConnectorConfirmation(runtimeLog?.pending_connector_confirmation, {
        digitalHumanId,
        run,
      }),
    [runtimeLog?.pending_connector_confirmation, digitalHumanId, run]
  );
  const submitConnectorDecision = useCallback<ConnectorRuntimeDecisionHandler>(
    async (command, scope) => {
      const outcome = await submitConnectorRuntimeDecision(command, scope);
      // A decision resumes or ends the durable run; refresh every surface that
      // still projects the waiting state so the drawer cannot offer a stale
      // approval. The transport one-shot guard owns duplicate-submit safety.
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.run(digitalHumanId, runId) }),
        queryClient.invalidateQueries({
          queryKey: DIGITAL_HUMAN_KEYS.runRuntimeLog(digitalHumanId, runId),
        }),
        queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.runs(digitalHumanId) }),
      ]);
      return outcome;
    },
    [digitalHumanId, runId, queryClient]
  );
  const submitWorkflowApproval = useCallback(
    async ({ inputs, action }: { inputs: Record<string, unknown>; action: string }) => {
      if (
        !run?.conversation_id ||
        !run.message_id ||
        !workflowApproval?.approval_token ||
        !workflowApprovalForm
      ) {
        return;
      }
      setWorkflowApprovalPending(true);
      try {
        const transport = new DigitalHumanRuntimeTransport(digitalHumanId);
        await transport.continueWorkflowApproval(
          run.conversation_id,
          run.message_id,
          {
            approvalToken: workflowApproval.approval_token,
            action,
            inputs,
          },
          discardWorkflowApprovalStream
        );
        await Promise.all([
          queryClient.invalidateQueries({
            queryKey: DIGITAL_HUMAN_KEYS.run(digitalHumanId, runId),
          }),
          queryClient.invalidateQueries({
            queryKey: DIGITAL_HUMAN_KEYS.runRuntimeLog(digitalHumanId, runId),
          }),
          queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.runs(digitalHumanId) }),
        ]);
      } finally {
        setWorkflowApprovalPending(false);
      }
    },
    [digitalHumanId, queryClient, run, runId, workflowApproval, workflowApprovalForm]
  );

  return (
    <AgentRuntimeLogDetailDrawer
      open={open}
      onOpenChange={onOpenChange}
      selectedRun={selectedRun}
      detail={runtimeLog?.detail ?? null}
      steps={runtimeSteps}
      isLoading={runQuery.isLoading || runtimeQuery.isLoading}
      error={error}
      sourceLabel={run ? runSourceLabel(run, t) : null}
      footer={
        run && active ? (
          <div className="flex flex-col gap-3">
            {connectorConfirmation ? (
              <div className="max-h-[45vh] overflow-y-auto pr-1">
                <ConnectorRuntimeConfirmationCard
                  key={connectorConfirmation.confirmation.route_receipt}
                  result={connectorConfirmation}
                  onDecision={submitConnectorDecision}
                  compact
                  showPreview
                  requireCompletePreview
                  previewLabels={{
                    accountName: t('runs.connectorConfirmation.accountName'),
                    accountIdentity: t('runs.connectorConfirmation.accountIdentity'),
                    unavailable: t('runs.connectorConfirmation.previewUnavailable'),
                  }}
                />
              </div>
            ) : workflowApprovalForm && workflowApproval?.approval_token ? (
              <div className="max-h-[55vh] overflow-y-auto rounded-md border bg-background p-3">
                <ApprovalRuntimeForm
                  form={workflowApprovalForm}
                  isSubmitting={workflowApprovalPending}
                  onSubmit={submitWorkflowApproval}
                />
              </div>
            ) : approvalWait && !runtimeQuery.isLoading ? (
              <div className="rounded-md border border-warning/25 bg-warning/10 px-3 py-2 text-xs leading-5">
                <div className="font-medium text-foreground">
                  {t('runs.connectorConfirmation.unavailableTitle')}
                </div>
                <div className="mt-1 text-muted-foreground">
                  {approvalReceipt
                    ? t('runs.connectorConfirmation.unavailableDescription')
                    : t('runs.connectorConfirmation.unavailableNoReceipt')}
                </div>
              </div>
            ) : null}
            <div className="flex justify-end">
              <Button
                variant="outline"
                onClick={() => void onCancel(run.id)}
                disabled={cancelPending || run.status === 'canceling'}
              >
                {cancelPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <Ban className="size-4" />
                )}
                {run.status === 'canceling' ? t('runs.canceling') : t('runs.cancel')}
              </Button>
            </div>
          </div>
        ) : (
          <DigitalHumanBackgroundRunFiles digitalHumanId={digitalHumanId} run={run} />
        )
      }
    />
  );
}

function toRuntimeRunItem(run: DigitalHumanRun): AgentRuntimeRunItem {
  return {
    id: run.message_id || run.id,
    conversation_id: run.conversation_id || '',
    status: run.status,
    query: run.query,
    answer_preview: run.result_summary,
    elapsed_time: runElapsedMilliseconds(run),
    created_at: unixSeconds(run.created_at),
    finished_at: run.completed_at ? unixSeconds(run.completed_at) : null,
    source: normalizedRunSource(run),
  };
}

function runElapsedMilliseconds(run: DigitalHumanRun) {
  const start = Date.parse(run.started_at ?? run.created_at);
  const end = Date.parse(run.completed_at ?? run.updated_at);
  if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) return undefined;
  return end - start;
}

function unixSeconds(value: string) {
  const timestamp = Date.parse(value);
  return Number.isFinite(timestamp) ? Math.floor(timestamp / 1_000) : undefined;
}

type DigitalHumanTranslator = ReturnType<typeof useT<'digitalHumans'>>;

function runSourceLabel(run: DigitalHumanRun, t: DigitalHumanTranslator) {
  return t(`runs.sources.${normalizedRunSource(run)}`);
}

function normalizedRunSource(run: DigitalHumanRun) {
  const origin = isRecord(run.invocation_event?.origin) ? run.invocation_event.origin : null;
  const source =
    (typeof origin?.kind === 'string' && origin.kind) ||
    (typeof run.invocation_event?.type === 'string' && run.invocation_event.type) ||
    'manual';
  if (source === 'schedule' || source === 'scheduled') {
    return digitalHumanRunStartedByPerson(run) ? ('manual' as const) : ('scheduled' as const);
  }
  if (source === 'email') return 'email' as const;
  if (source === 'channel' || source === 'chat') return 'channel' as const;
  if (source === 'manual') return 'manual' as const;
  return 'external' as const;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}
