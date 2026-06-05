'use client';

import { useEffect, useMemo, useState, type ReactNode } from 'react';
import JsonView from '@uiw/react-json-view';
import { lightTheme } from '@uiw/react-json-view/light';
import {
  AlertCircle,
  Bot,
  BookOpenText,
  ChevronRight,
  Clock,
  Hash,
  MessageSquareText,
  ShieldAlert,
  Sparkles,
  Wrench,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { useT } from '@/i18n/translations';
import type {
  AgentRuntimeRunDetail,
  AgentRuntimeRunItem,
  AgentRuntimeStep,
} from '@/services/types/agent-runtime-log';
import { cn } from '@/lib/utils';
import { formatDate, formatWorkflowElapsedMs } from '@/utils/format';
import { getAgentRuntimeStepDisplay } from './agent-runtime-step-display';
import { LogStatusBadge } from './log-status-badge';

interface AgentRuntimeLogDetailDrawerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  selectedRun: AgentRuntimeRunItem | null;
  detail: AgentRuntimeRunDetail | null;
  steps: AgentRuntimeStep[];
  isLoading: boolean;
  error?: string | null;
}

const AGENT_RUNTIME_HIDDEN_SKILL_INSTRUCTIONS = '__ZGI_HIDDEN_SKILL_INSTRUCTIONS__';

interface AgentRuntimeHiddenValueLabels {
  hiddenSkillInstructions: string;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isEmptyValue(value: unknown): boolean {
  if (value === null || value === undefined || value === '') return true;
  if (Array.isArray(value)) return value.length === 0;
  if (isRecord(value)) return Object.keys(value).length === 0;
  return false;
}

function numberValue(value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string' && value.trim() !== '') {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : null;
  }
  return null;
}

function stepTotalTokens(step: AgentRuntimeStep | null): number | null {
  if (!step || !isRecord(step.process)) return null;
  const direct = numberValue(step.process.total_tokens);
  if (direct !== null) return direct;
  const usage = step.process.usage;
  if (!isRecord(usage)) return null;
  return numberValue(usage.total_tokens);
}

function localizeHiddenRuntimeString(value: string, labels: AgentRuntimeHiddenValueLabels): string {
  if (value === AGENT_RUNTIME_HIDDEN_SKILL_INSTRUCTIONS) {
    return labels.hiddenSkillInstructions;
  }
  return value;
}

function localizeHiddenRuntimeValue(
  value: unknown,
  labels: AgentRuntimeHiddenValueLabels
): unknown {
  if (typeof value === 'string') {
    return localizeHiddenRuntimeString(value, labels);
  }
  if (Array.isArray(value)) {
    return value.map(item => localizeHiddenRuntimeValue(item, labels));
  }
  if (isRecord(value)) {
    return Object.fromEntries(
      Object.entries(value).map(([key, item]) => [key, localizeHiddenRuntimeValue(item, labels)])
    );
  }
  return value;
}

function renderValue(value: unknown, labels: AgentRuntimeHiddenValueLabels) {
  if (isEmptyValue(value)) {
    return <div className="text-sm text-muted-foreground">-</div>;
  }

  if (typeof value === 'string') {
    return (
      <div className="whitespace-pre-wrap break-words text-sm leading-6">
        {localizeHiddenRuntimeString(value, labels)}
      </div>
    );
  }

  const displayValue = localizeHiddenRuntimeValue(value, labels);
  return (
    <div className="max-h-[360px] overflow-auto rounded-md bg-muted/40 p-3 text-xs">
      <JsonView
        value={displayValue as object}
        style={{ ...lightTheme, background: 'transparent' }}
        className="text-xs"
      />
    </div>
  );
}

function renderJsonValue(value: unknown, labels: AgentRuntimeHiddenValueLabels) {
  const displayValue = localizeHiddenRuntimeValue(value, labels);
  return (
    <div className="max-h-[360px] overflow-auto rounded-md bg-muted/40 p-3 text-xs">
      <JsonView
        value={displayValue as object}
        style={{ ...lightTheme, background: 'transparent' }}
        className="text-xs"
      />
    </div>
  );
}

function parseJsonString(value: string): unknown {
  const trimmed = value.trim();
  if (!trimmed) return value;
  try {
    return JSON.parse(trimmed);
  } catch {
    return value;
  }
}

function contentSize(value: unknown): number {
  if (typeof value === 'string') return value.length;
  try {
    return JSON.stringify(value).length;
  } catch {
    return 0;
  }
}

function modelRequestWithoutMessages(value: Record<string, unknown>) {
  const { messages: _messages, ...rest } = value;
  return rest;
}

function roleLabel(role: unknown): string {
  return typeof role === 'string' && role.trim() ? role.trim() : '-';
}

function renderModelMessage(message: unknown, index: number, labels: AgentRuntimeModelInputLabels) {
  if (!isRecord(message)) {
    return (
      <div key={index} className="rounded-md border p-3">
        {renderValue(message, labels)}
      </div>
    );
  }

  const role = roleLabel(message.role);
  if (role === 'tool') {
    const content = message.content;
    const displayContent = typeof content === 'string' ? parseJsonString(content) : content;
    const toolCallID = typeof message.tool_call_id === 'string' ? message.tool_call_id : '';
    return (
      <details key={index} className="group rounded-md border bg-muted/20">
        <summary className="flex cursor-pointer list-none items-center gap-2 px-3 py-2 text-sm">
          <ChevronRight className="size-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-90" />
          <span className="font-medium">{labels.toolResult}</span>
          <span className="min-w-0 truncate text-xs text-muted-foreground">
            {toolCallID ? labels.toolResultWithID(toolCallID) : labels.toolResultMeta}
            {' · '}
            {labels.toolResultSize(contentSize(content))}
          </span>
        </summary>
        <div className="border-t p-3">{renderValue(displayContent, labels)}</div>
      </details>
    );
  }

  return (
    <div key={index} className="rounded-md border">
      <div className="border-b bg-muted/20 px-3 py-2 text-xs font-medium text-muted-foreground">
        {labels.messageRole(role)}
      </div>
      <div className="p-3">{renderJsonValue(message, labels)}</div>
    </div>
  );
}

interface AgentRuntimeModelInputLabels extends AgentRuntimeHiddenValueLabels {
  requestParams: string;
  messages: string;
  toolResult: string;
  toolResultMeta: string;
  messageRole: (role: string) => string;
  toolResultWithID: (id: string) => string;
  toolResultSize: (chars: number) => string;
}

function renderModelInput(value: unknown, labels: AgentRuntimeModelInputLabels) {
  if (!isRecord(value) || !Array.isArray(value.messages)) {
    return renderValue(value, labels);
  }

  const requestParams = modelRequestWithoutMessages(value);
  return (
    <div className="space-y-4">
      {!isEmptyValue(requestParams) ? (
        <div>
          <div className="mb-2 text-xs font-medium text-muted-foreground">
            {labels.requestParams}
          </div>
          {renderJsonValue(requestParams, labels)}
        </div>
      ) : null}
      <div>
        <div className="mb-2 text-xs font-medium text-muted-foreground">{labels.messages}</div>
        <div className="space-y-2">
          {value.messages.map((message, index) => renderModelMessage(message, index, labels))}
        </div>
      </div>
    </div>
  );
}

function stepIcon(type: string) {
  if (type === 'user_input') return MessageSquareText;
  if (type === 'model_call') return Bot;
  if (type === 'model_answer') return Bot;
  if (type === 'tool_call' || type === 'tool') return Wrench;
  if (type === 'skill_load' || type === 'skill') return Sparkles;
  if (type === 'reference_read') return BookOpenText;
  if (type === 'guardrail') return ShieldAlert;
  return Hash;
}

function DetailMetric({
  label,
  value,
}: {
  label: string;
  value: string | number | null | undefined;
}) {
  return (
    <div className="min-w-0">
      <div className="text-xs font-medium text-muted-foreground">{label}</div>
      <div className="mt-1 truncate text-sm text-foreground" title={value ? String(value) : '-'}>
        {value ?? '-'}
      </div>
    </div>
  );
}

function DetailSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="border-b px-5 py-4 last:border-b-0">
      <div className="mb-3 text-sm font-semibold">{title}</div>
      {children}
    </section>
  );
}

export function AgentRuntimeLogDetailDrawer({
  open,
  onOpenChange,
  selectedRun,
  detail,
  steps,
  isLoading,
  error,
}: AgentRuntimeLogDetailDrawerProps) {
  const t = useT('webapp');
  const tAgents = useT('agents');
  const tCommon = useT('common');
  const [selectedStepId, setSelectedStepId] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setSelectedStepId(prev => {
      if (prev && steps.some(step => step.id === prev)) return prev;
      return steps[0]?.id ?? null;
    });
  }, [open, steps]);

  const selectedStep = useMemo(
    () => steps.find(step => step.id === selectedStepId) ?? steps[0] ?? null,
    [selectedStepId, steps]
  );
  const runId = detail?.id ?? selectedRun?.id ?? null;
  const status = detail?.status ?? selectedRun?.status;
  const createdAt = detail?.created_at ?? selectedRun?.created_at;
  const finishedAt = detail?.finished_at ?? selectedRun?.finished_at;
  const elapsedTime = detail?.elapsed_time ?? selectedRun?.elapsed_time;
  const totalTokens = detail?.total_tokens ?? selectedRun?.total_tokens;
  const totalSteps = detail?.total_steps ?? selectedRun?.total_steps ?? steps.length;
  const userInput = detail?.query ?? selectedRun?.query ?? '';
  const selectedStepTotalTokens = stepTotalTokens(selectedStep);
  const selectedStepDisplay = selectedStep ? getAgentRuntimeStepDisplay(selectedStep, t) : null;
  const modelInputLabels = useMemo<AgentRuntimeModelInputLabels>(
    () => ({
      hiddenSkillInstructions: t('appLogs.hiddenValues.skillInstructions'),
      requestParams: t('appLogs.runtimeModelRequestParams'),
      messages: t('appLogs.runtimeModelMessages'),
      toolResult: t('appLogs.runtimeToolResultMessage'),
      toolResultMeta: t('appLogs.runtimeToolResultMeta'),
      messageRole: role => t('appLogs.runtimeModelMessageRole', { role }),
      toolResultWithID: id => t('appLogs.runtimeToolResultWithID', { id }),
      toolResultSize: chars => t('appLogs.runtimeToolResultSize', { chars }),
    }),
    [t]
  );

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        showClose={false}
        className="flex h-full w-screen max-w-none flex-col gap-0 p-0 md:w-[80vw] sm:max-w-none"
      >
        <SheetHeader className="shrink-0 border-b px-5 py-4 text-left">
          <div className="flex min-w-0 items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <SheetTitle className="text-base">{t('appLogs.runtimeDialogTitle')}</SheetTitle>
                {status ? <LogStatusBadge status={status} /> : null}
              </div>
              <SheetDescription className="mt-1 truncate" title={runId ?? ''}>
                {runId
                  ? t('appLogs.runtimeDialogDescription', { id: runId })
                  : t('appLogs.selectRunDescription')}
              </SheetDescription>
            </div>
            <SheetClose asChild>
              <Button type="button" variant="outline" size="xs" className="shrink-0">
                {tCommon('close')}
              </Button>
            </SheetClose>
          </div>
        </SheetHeader>

        <div className="grid shrink-0 grid-cols-2 gap-3 border-b px-5 py-4 md:grid-cols-6">
          <DetailMetric
            label={t('appLogs.columns.createdAt')}
            value={createdAt ? formatDate(createdAt) : null}
          />
          <DetailMetric
            label={tAgents('workflow.finishedAt')}
            value={finishedAt ? formatDate(finishedAt) : null}
          />
          <DetailMetric
            label={tAgents('workflow.elapsed')}
            value={typeof elapsedTime === 'number' ? formatWorkflowElapsedMs(elapsedTime) : null}
          />
          <DetailMetric label={tAgents('workflow.tokens')} value={totalTokens} />
          <DetailMetric label={tAgents('workflow.steps')} value={totalSteps} />
          <DetailMetric
            label={t('appLogs.columns.conversation')}
            value={detail?.conversation_id ?? selectedRun?.conversation_id}
          />
        </div>

        {error ? (
          <div className="mx-5 mt-4 shrink-0 rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">
            {error}
          </div>
        ) : null}

        <div className="grid min-h-0 flex-1 grid-cols-[320px_minmax(0,1fr)] overflow-hidden">
          <aside className="min-h-0 border-r">
            <div className="border-b px-4 py-3 text-sm font-semibold">
              {t('appLogs.runtimeSteps')}
            </div>
            <div className="h-full min-h-0 overflow-auto p-3">
              {isLoading && steps.length === 0 ? (
                <div className="flex items-center gap-2 p-3 text-sm text-muted-foreground">
                  <Clock className="size-4 animate-pulse" />
                  {tAgents('loading')}
                </div>
              ) : steps.length === 0 ? (
                <div className="p-3 text-sm text-muted-foreground">
                  {t('appLogs.runtimeNoStep')}
                </div>
              ) : (
                <div className="space-y-1.5">
                  {steps.map(step => {
                    const Icon = stepIcon(step.type);
                    const selected = selectedStep?.id === step.id;
                    const display = getAgentRuntimeStepDisplay(step, t);
                    return (
                      <button
                        key={step.id}
                        type="button"
                        className={cn(
                          'flex w-full min-w-0 items-start gap-2 rounded-md border px-3 py-2 text-left transition-colors',
                          selected
                            ? 'border-primary/30 bg-primary/5'
                            : 'border-transparent hover:bg-muted/60'
                        )}
                        onClick={() => setSelectedStepId(step.id)}
                      >
                        <span className="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-md bg-muted">
                          <Icon className="size-3.5 text-muted-foreground" />
                        </span>
                        <span className="min-w-0 flex-1">
                          <span className="block truncate text-sm font-medium">
                            {display.title}
                          </span>
                          <span className="mt-0.5 block truncate text-xs text-muted-foreground">
                            {display.subtitle || step.type}
                            {typeof step.elapsed_time === 'number'
                              ? ` - ${formatWorkflowElapsedMs(step.elapsed_time)}`
                              : ''}
                          </span>
                        </span>
                        <LogStatusBadge status={step.status} />
                      </button>
                    );
                  })}
                </div>
              )}
            </div>
          </aside>

          <main className="min-h-0 overflow-auto">
            {!selectedStep ? (
              <div className="flex h-full items-center justify-center gap-2 text-sm text-muted-foreground">
                <AlertCircle className="size-4" />
                {t('appLogs.runtimeNoStep')}
              </div>
            ) : (
              <>
                <DetailSection title={t('appLogs.runtimeUserInput')}>
                  {userInput ? renderValue(userInput, modelInputLabels) : t('appLogs.noQuery')}
                </DetailSection>

                <DetailSection title={t('appLogs.runtimeStepDetails')}>
                  <div className="grid grid-cols-2 gap-3 md:grid-cols-5">
                    <DetailMetric label={t('appLogs.runtimeType')} value={selectedStep.type} />
                    <DetailMetric
                      label={t('appLogs.runtimeName')}
                      value={selectedStepDisplay?.title}
                    />
                    <DetailMetric
                      label={tAgents('workflow.elapsed')}
                      value={
                        typeof selectedStep.elapsed_time === 'number'
                          ? formatWorkflowElapsedMs(selectedStep.elapsed_time)
                          : null
                      }
                    />
                    <DetailMetric
                      label={tAgents('workflow.tokens')}
                      value={selectedStepTotalTokens}
                    />
                    <DetailMetric
                      label={t('appLogs.columns.createdAt')}
                      value={selectedStep.created_at ? formatDate(selectedStep.created_at) : null}
                    />
                  </div>
                </DetailSection>

                <DetailSection title={t('appLogs.runtimeInput')}>
                  {selectedStep.type === 'model_call'
                    ? renderModelInput(selectedStep.input, modelInputLabels)
                    : renderValue(selectedStep.input, modelInputLabels)}
                </DetailSection>

                <DetailSection title={t('appLogs.runtimeOutput')}>
                  {renderValue(selectedStep.output, modelInputLabels)}
                </DetailSection>

                {!isEmptyValue(selectedStep.process) ? (
                  <DetailSection
                    title={
                      selectedStep.type === 'model_answer' || selectedStep.type === 'model_call'
                        ? t('appLogs.runtimeModelInfo')
                        : t('appLogs.runtimeToolInfo')
                    }
                  >
                    {renderValue(selectedStep.process, modelInputLabels)}
                  </DetailSection>
                ) : null}

                {selectedStep.error ? (
                  <DetailSection title={t('appLogs.runtimeError')}>
                    <div className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">
                      {selectedStep.error}
                    </div>
                  </DetailSection>
                ) : null}

                {detail?.error && selectedStep.type === 'model_answer' ? (
                  <DetailSection title={t('appLogs.runtimeError')}>
                    <div className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">
                      {detail.error}
                    </div>
                  </DetailSection>
                ) : null}
              </>
            )}
          </main>
        </div>
      </SheetContent>
    </Sheet>
  );
}
