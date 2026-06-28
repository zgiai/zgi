import React from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import { Loader2 } from 'lucide-react';
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
import { ApprovalCompletedState } from '@/components/workflow/approval/approval-completed-state';
import ApprovalRuntimeForm from '@/components/workflow/approval/approval-runtime-form';
import {
  isApprovalFormAlreadySubmittedError,
  type ApprovalRuntimeForm as ApprovalRuntimeFormData,
} from '@/services/approval.service';
import { Button } from '@/components/ui/button';
import { QuestionAnswerRuntimePrompt } from '@/components/workflow/question-answer/question-answer-runtime-prompt';
import type { QuestionAnswerChoice } from '@/services/types/workflow';
import type { QuestionAnswerTranscriptItem } from '@/components/workflow/question-answer/runtime-events';
import DebugInputGuide, { type DebugSampleInput } from './debug-input-guide';

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
  const questionAnswerContent = questionAnswerPrompt ? (
    <QuestionAnswerRuntimePrompt
      question={questionAnswerPrompt.question}
      choices={questionAnswerPrompt.choices}
      round={questionAnswerPrompt.round}
      submitting={questionAnswerSubmitting}
      onSelectChoice={onQuestionAnswerSelect}
    />
  ) : null;
  const hasInputs = startVariables.length > 0;
  const hasRunState =
    runItems.length > 0 || Boolean(runSummary) || Boolean(streamedText) || Boolean(finalResult);
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
            <div className="mb-4 flex min-h-[180px] items-center justify-center rounded-lg border bg-card p-3">
              <Loader2 className="size-5 animate-spin text-muted-foreground" />
            </div>
          ) : approvalExpired ? (
            <ApprovalCompletedState compact variant="expired" className="mb-4" />
          ) : approvalAlreadyCompleted ? (
            <ApprovalCompletedState compact className="mb-4" />
          ) : approvalError ? (
            <div className="mb-4 rounded-lg border bg-card p-4 text-center">
              <div className="text-sm font-medium">{t('nodes.approval.runtime.loadFailed')}</div>
              <p className="mt-2 text-xs text-muted-foreground">
                {approvalError instanceof Error
                  ? approvalError.message
                  : t('nodes.approval.runtime.loadFailedDescription')}
              </p>
              {onApprovalRetry ? (
                <Button type="button" size="sm" className="mt-3" onClick={onApprovalRetry}>
                  {t('nodes.approval.runtime.retry')}
                </Button>
              ) : null}
            </div>
          ) : approvalForm && onApprovalSubmit ? (
            <div className="mb-4 rounded-lg border bg-card p-3">
              <ApprovalRuntimeForm
                form={approvalForm}
                onSubmit={onApprovalSubmit}
                isSubmitting={approvalSubmitting}
                submittedAction={approvalSubmittedAction}
              />
            </div>
          ) : null}
          <div className="min-h-[240px]">
            <Results
              mode="draft"
              streamedText={streamedText}
              historyResult={finalResult}
              emptyText={t('agents.workflow.noOutputYet')}
              questionAnswerTranscript={questionAnswerTranscript}
            />
          </div>
        </TabsContent>
      </div>
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
    </Tabs>
  );
};

export default DraftContent;
