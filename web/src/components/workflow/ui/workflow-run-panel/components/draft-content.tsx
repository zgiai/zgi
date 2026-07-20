import React from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import { AlertTriangle, Loader2 } from 'lucide-react';
import ExecutionTab from '@/components/workflow/ui/workflow-run-panel/components/workflow-run-panel-execution';
import InputsTab from '@/components/workflow/ui/workflow-run-panel/components/workflow-run-panel-inputs';
import DetailsTab from '@/components/workflow/ui/workflow-run-panel/components/workflow-run-panel-details';
import type { WorkflowRunNodeListItem } from '@/components/workflow/ui/workflow-run-nodes-list';
import type { InputVar } from '@/components/workflow/types/input-var';
import type {
  FormInputs,
  WorkflowInputFormHandle,
} from '@/components/workflow/common/workflow-input-form';
import { getInputVarSchemaDefaultValue } from '@/components/workflow/common/workflow-input-form';
import type { WorkflowFinishedData, HistoryResult } from '../types';
import Results from './results';
import { useT } from '@/i18n';
import WorkflowApprovalInteractionCard from '@/components/workflow/approval/workflow-approval-interaction-card';
import {
  isApprovalFormAlreadySubmittedError,
  type ApprovalRuntimeForm as ApprovalRuntimeFormData,
} from '@/services/approval.service';
import { Button } from '@/components/ui/button';
import { QuestionAnswerRuntimePrompt } from '@/components/workflow/question-answer/question-answer-runtime-prompt';
import type { QuestionAnswerChoice } from '@/services/types/workflow';
import type { QuestionAnswerTranscriptItem } from '@/components/workflow/question-answer/runtime-events';
import DebugInputGuide, { type DebugSampleInput } from './debug-input-guide';
import { WorkflowRuntimeStopAction } from '@/components/workflow/runtime/workflow-runtime-stop-action';

interface DraftContentProps {
  activeTab: 'inputs' | 'execution' | 'details' | 'results';
  setActiveTab: (t: 'inputs' | 'execution' | 'details' | 'results') => void;
  isLoadingDraft: boolean;
  hasLocalNodes: boolean;
  startVariables: InputVar[];
  initialValues?: FormInputs;
  isStarting: boolean;
  isRunning?: boolean;
  isStopping?: boolean;
  runDisabled?: boolean;
  runDisabledMessage?: string;
  stopDisabled?: boolean;
  stopDisabledMessage?: string;
  onSubmit: (values: FormInputs) => void;
  onRunNoInputs: () => void;
  onInputChange?: (values: FormInputs) => void;
  onStop?: () => void;
  inputTopNotice?: React.ReactNode;
  debugSetupHints?: string[];
  runItems: WorkflowRunNodeListItem[];
  runSummary: WorkflowFinishedData | null;
  streamedText: string;
  finalResult?: HistoryResult | null;
  approvalForm?: ApprovalRuntimeFormData | null;
  approvalLoading?: boolean;
  approvalError?: unknown;
  approvalExpired?: boolean;
  onApprovalRetry?: () => void;
  approvalSubmitting?: boolean;
  approvalSubmittedAction?: string | null;
  onApprovalSubmit?: (payload: { inputs: Record<string, unknown>; action: string }) => void;
  questionAnswerPrompt?: {
    question: string;
    choices: QuestionAnswerChoice[];
    round?: number;
  } | null;
  questionAnswerSubmitting?: boolean;
  onQuestionAnswerSelect?: (choice: QuestionAnswerChoice) => void;
  questionAnswerTranscript?: QuestionAnswerTranscriptItem[];
}

function hasMeaningfulResult(result?: HistoryResult | null): boolean {
  if (!result || result.kind === 'empty') return false;
  if (result.kind === 'text') return result.content.trim().length > 0;

  const value = result.value;
  if (value === null || value === undefined) return false;
  if (Array.isArray(value)) return value.length > 0;
  if (typeof value === 'object') return Object.keys(value as Record<string, unknown>).length > 0;
  return true;
}

const DraftContent: React.FC<DraftContentProps> = ({
  activeTab,
  setActiveTab,
  isLoadingDraft,
  hasLocalNodes,
  startVariables,
  initialValues,
  isStarting,
  isRunning = false,
  isStopping = false,
  runDisabled = false,
  runDisabledMessage,
  stopDisabled = false,
  stopDisabledMessage,
  onSubmit,
  onRunNoInputs,
  onInputChange,
  onStop,
  inputTopNotice,
  debugSetupHints = [],
  runItems,
  runSummary,
  streamedText,
  finalResult,
  approvalForm,
  approvalLoading,
  approvalError,
  approvalExpired,
  onApprovalRetry,
  approvalSubmitting,
  approvalSubmittedAction,
  onApprovalSubmit,
  questionAnswerPrompt,
  questionAnswerSubmitting,
  onQuestionAnswerSelect,
  questionAnswerTranscript,
}) => {
  const t = useT();
  const inputFormRef = React.useRef<WorkflowInputFormHandle>(null);
  const approvalAlreadyCompleted = isApprovalFormAlreadySubmittedError(approvalError);
  const hasPendingInteraction = Boolean(
    questionAnswerPrompt ||
      approvalForm ||
      approvalLoading ||
      (approvalError && !approvalAlreadyCompleted && !approvalExpired)
  );
  const interactionStopAction = hasPendingInteraction ? (
    <WorkflowRuntimeStopAction
      onStop={onStop}
      isStopping={isStopping}
      disabled={stopDisabled || approvalSubmitting || questionAnswerSubmitting}
    />
  ) : null;
  const questionAnswerContent = questionAnswerPrompt ? (
    <QuestionAnswerRuntimePrompt
      question={questionAnswerPrompt.question}
      choices={questionAnswerPrompt.choices}
      round={questionAnswerPrompt.round}
      submitting={questionAnswerSubmitting}
      onSelectChoice={onQuestionAnswerSelect}
      secondaryAction={interactionStopAction}
    />
  ) : null;
  const hasInputs = startVariables.length > 0;
  const hasRunState =
    runItems.length > 0 || Boolean(runSummary) || Boolean(streamedText) || Boolean(finalResult);
  const runStatus = String(runSummary?.status ?? '').toLowerCase();
  const hasVisibleResult =
    streamedText.trim().length > 0 ||
    hasMeaningfulResult(finalResult) ||
    Boolean(questionAnswerTranscript?.length);
  const isWaitingForOutput = isRunning && !hasVisibleResult && !hasPendingInteraction;
  const shouldHideEmptyResult = hasPendingInteraction && !hasVisibleResult;
  const hasPartialFailedResult =
    (runStatus === 'failed' || runStatus === 'error') &&
    (streamedText.trim().length > 0 || Boolean(finalResult));
  const isPrimaryActionDisabled = runDisabled || isStarting || (isLoadingDraft && !hasLocalNodes);
  const debugSample = React.useMemo<DebugSampleInput | null>(() => {
    const values: FormInputs = {};
    const previewItems: DebugSampleInput['previewItems'] = [];

    startVariables.forEach(input => {
      if (input.type === 'file' || input.type === 'file-list') return;
      const defaultValue = getInputVarSchemaDefaultValue(input);
      if (defaultValue === undefined || defaultValue === null) return;
      if (typeof defaultValue === 'string' && defaultValue.trim().length === 0) return;
      if (typeof defaultValue === 'number' && !Number.isFinite(defaultValue)) return;

      values[input.variable] = defaultValue as FormInputs[string];
      if (previewItems.length < 2) {
        const text =
          typeof defaultValue === 'boolean'
            ? t(
                defaultValue
                  ? 'agents.workflow.debugGuide.booleanTrue'
                  : 'agents.workflow.debugGuide.booleanFalse'
              )
            : String(defaultValue).replace(/\s+/g, ' ').trim();
        previewItems.push({
          label: input.label || input.variable,
          value: text.length > 90 ? `${text.slice(0, 90)}...` : text,
        });
      }
    });

    if (Object.keys(values).length === 0) return null;

    return {
      title: t('agents.workflow.debugGuide.sampleTitle'),
      description: t('agents.workflow.debugGuide.sampleDescription'),
      values,
      previewItems,
    };
  }, [startVariables, t]);
  const shouldShowRestoreDefaults =
    hasInputs &&
    (Boolean(debugSample) ||
      Boolean(initialValues && Object.values(initialValues).some(value => value !== undefined)));

  const handlePrimaryAction = () => {
    if (isRunning) {
      if (isStopping || stopDisabled) return;
      onStop?.();
      return;
    }

    if (hasInputs) {
      inputFormRef.current?.submit();
      return;
    }

    onRunNoInputs();
  };

  const handleResetInputs = () => {
    inputFormRef.current?.reset();
    setActiveTab('inputs');
  };

  const handleApplySample = () => {
    if (!debugSample) return;
    inputFormRef.current?.setValues(debugSample.values);
    onInputChange?.(debugSample.values);
    setActiveTab('inputs');
  };

  const hasInputGuide = Boolean(debugSample) || debugSetupHints.length > 0;
  const inputGuide = hasInputGuide ? (
    <DebugInputGuide
      sample={debugSample}
      setupHints={debugSetupHints}
      onApplySample={handleApplySample}
    />
  ) : null;
  const inputTopContent =
    inputTopNotice || inputGuide ? (
      <>
        {inputTopNotice}
        {inputGuide}
      </>
    ) : undefined;

  return (
    <Tabs
      value={activeTab}
      onValueChange={v => setActiveTab(v as 'inputs' | 'execution' | 'details' | 'results')}
      className="flex flex-col h-full"
    >
      <div className="px-4 pt-4 shrink-0">
        <TabsList className="w-full">
          <TabsTrigger className="flex-1" value="inputs">
            {t('agents.workflow.inputs')}
          </TabsTrigger>
          <TabsTrigger className="flex-1" value="execution">
            {t('agents.workflow.execution')}
          </TabsTrigger>
          <TabsTrigger className="flex-1" value="details">
            {t('agents.workflow.details')}
          </TabsTrigger>
          <TabsTrigger className="flex-1" value="results">
            {t('agents.workflow.results')}
          </TabsTrigger>
        </TabsList>
      </div>
      <div className="h-0 grow">
        <TabsContent
          value="execution"
          className="h-full overflow-y-auto px-4 pb-4 mt-3 outline-none"
        >
          <ExecutionTab items={runItems} />
        </TabsContent>

        <TabsContent
          forceMount
          value="inputs"
          className="h-full overflow-y-auto px-4 pb-4 mt-3 outline-none data-[state=inactive]:hidden"
        >
          <InputsTab
            isLoadingDraft={isLoadingDraft}
            hasLocalNodes={hasLocalNodes}
            startVariables={startVariables}
            initialValues={initialValues}
            isStarting={isStarting}
            onSubmit={onSubmit}
            onRunNoInputs={onRunNoInputs}
            topContent={questionAnswerContent}
            onInputChange={onInputChange}
            formRef={inputFormRef}
            hideSubmitButton
            topNotice={inputTopContent}
          />
        </TabsContent>

        <TabsContent value="details" className="h-full overflow-y-auto px-4 pb-4 mt-3 outline-none">
          <DetailsTab runSummary={runSummary} />
        </TabsContent>

        <TabsContent value="results" className="h-full overflow-y-auto px-4 pb-4 mt-3 outline-none">
          {approvalLoading ? (
            <WorkflowApprovalInteractionCard
              mode="loading"
              className="mb-4"
              secondaryAction={interactionStopAction}
            />
          ) : approvalExpired ? (
            <WorkflowApprovalInteractionCard mode="expired" className="mb-4" />
          ) : approvalAlreadyCompleted ? (
            <WorkflowApprovalInteractionCard mode="completed" className="mb-4" />
          ) : approvalError ? (
            <WorkflowApprovalInteractionCard
              mode="error"
              className="mb-4"
              error={approvalError}
              onRetry={onApprovalRetry}
              secondaryAction={interactionStopAction}
            />
          ) : approvalForm && onApprovalSubmit ? (
            <WorkflowApprovalInteractionCard
              mode={approvalSubmittedAction ? 'submitted' : 'form'}
              className="mb-4"
              form={approvalForm}
              onSubmit={onApprovalSubmit}
              isSubmitting={approvalSubmitting}
              submittedAction={approvalSubmittedAction}
              secondaryAction={interactionStopAction}
            />
          ) : null}
          {hasPartialFailedResult ? (
            <div className="mb-3 flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">
              <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
              <span>{t('agents.workflow.partialResultAfterFailure')}</span>
            </div>
          ) : null}
          {!shouldHideEmptyResult ? (
            <div className="min-h-[240px]">
              {isWaitingForOutput ? (
                <div className="flex min-h-[240px] flex-col items-center justify-center gap-3 text-muted-foreground">
                  <div className="flex size-10 items-center justify-center rounded-full bg-muted/60">
                    <Loader2 className="size-5 animate-spin text-primary/70" />
                  </div>
                  <span className="text-sm">{t('agents.workflow.waitingForOutput')}</span>
                </div>
              ) : (
                <Results
                  mode="draft"
                  streamedText={streamedText}
                  historyResult={finalResult}
                  emptyText={t('agents.workflow.noOutputYet')}
                  questionAnswerTranscript={questionAnswerTranscript}
                />
              )}
            </div>
          ) : null}
        </TabsContent>
      </div>
      {!hasPendingInteraction ? (
        <div className="shrink-0 border-t bg-background/95 px-4 py-3 backdrop-blur">
          <div className="flex items-center justify-between gap-2">
            <Button
              type="button"
              size="sm"
              variant={isRunning ? 'destructive' : 'default'}
              className="h-9 min-w-[104px] rounded-md font-semibold"
              disabled={isRunning ? isStopping || stopDisabled : isPrimaryActionDisabled}
              onClick={handlePrimaryAction}
            >
              {isRunning
                ? t('agents.workflow.stop')
                : isStarting
                  ? t('agents.workflow.starting')
                  : hasRunState
                    ? t('agents.workflow.rerunDebug')
                    : t('agents.workflow.runNow')}
            </Button>
            {!isRunning && runDisabled && runDisabledMessage ? (
              <div className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
                {runDisabledMessage}
              </div>
            ) : isRunning && stopDisabled && stopDisabledMessage ? (
              <div className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
                {stopDisabledMessage}
              </div>
            ) : null}
            {shouldShowRestoreDefaults && (
              <Button
                type="button"
                size="sm"
                variant="ghost"
                className="h-9 rounded-md text-muted-foreground"
                disabled={isRunning || isStarting}
                onClick={handleResetInputs}
              >
                {t('agents.workflow.restoreDefaults')}
              </Button>
            )}
          </div>
        </div>
      ) : null}
    </Tabs>
  );
};

export default DraftContent;
