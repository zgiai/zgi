'use client';

import * as React from 'react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Checkbox } from '@/components/ui/checkbox';
import { ModelSelector } from '@/components/common/model-selector';
import type { ModelSelectorValue } from '@/components/common/model-selector';
import { useCreateWorkflowTestScenarioRecognitionTask } from '@/hooks/workflow-test/use-workflow-test';
import { useDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { useT } from '@/i18n';
import type { WorkflowTestMode } from './case-metadata';

interface RecognizeScenariosDialogProps {
  agentId: string;
  defaultContext: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode?: WorkflowTestMode;
}

const FOCUS_OPTIONS = [
  {
    value: 'high_frequency',
    labelKey: 'focusHighFrequency' as const,
    payloadKey: 'focusHighFrequency' as const,
    modes: ['conversation'] as WorkflowTestMode[],
  },
  {
    value: 'critical_flow',
    labelKey: 'focusCriticalFlow' as const,
    payloadKey: 'focusCriticalFlow' as const,
    modes: ['conversation'] as WorkflowTestMode[],
  },
  {
    value: 'fallback',
    labelKey: 'focusFallback' as const,
    payloadKey: 'focusFallback' as const,
    modes: ['conversation'] as WorkflowTestMode[],
  },
  {
    value: 'complaint',
    labelKey: 'focusComplaint' as const,
    payloadKey: 'focusComplaint' as const,
    modes: ['conversation'] as WorkflowTestMode[],
  },
  {
    value: 'human_handoff',
    labelKey: 'focusHumanHandoff' as const,
    payloadKey: 'focusHumanHandoff' as const,
    modes: ['conversation'] as WorkflowTestMode[],
  },
  {
    value: 'file_input',
    labelKey: 'focusFileInput' as const,
    payloadKey: 'focusFileInput' as const,
    modes: [] as WorkflowTestMode[],
  },
  {
    value: 'business_object',
    labelKey: 'taskFocusBusinessObject' as const,
    payloadKey: 'taskFocusBusinessObject' as const,
    modes: ['task'] as WorkflowTestMode[],
  },
  {
    value: 'processing_goal',
    labelKey: 'taskFocusProcessingGoal' as const,
    payloadKey: 'taskFocusProcessingGoal' as const,
    modes: ['task'] as WorkflowTestMode[],
  },
  {
    value: 'output_usage',
    labelKey: 'taskFocusOutputUsage' as const,
    payloadKey: 'taskFocusOutputUsage' as const,
    modes: ['task'] as WorkflowTestMode[],
  },
  {
    value: 'industry_context',
    labelKey: 'taskFocusIndustryContext' as const,
    payloadKey: 'taskFocusIndustryContext' as const,
    modes: ['task'] as WorkflowTestMode[],
  },
  {
    value: 'multi_turn_context',
    labelKey: 'focusMultiTurnContext' as const,
    payloadKey: 'focusMultiTurnContext' as const,
    modes: ['conversation'] as WorkflowTestMode[],
  },
];

const GRANULARITY_OPTIONS = [
  {
    value: 'merge_similar',
    labelKey: 'granularityMerge' as const,
    payloadKey: 'granularityMerge' as const,
  },
  {
    value: 'balanced',
    labelKey: 'granularityBalanced' as const,
    payloadKey: 'granularityBalanced' as const,
  },
  {
    value: 'fine_grained',
    labelKey: 'granularityFine' as const,
    payloadKey: 'granularityFine' as const,
  },
];

function defaultFocusValues(mode: WorkflowTestMode) {
  return mode === 'task'
    ? ['business_object', 'processing_goal', 'output_usage', 'industry_context']
    : ['high_frequency', 'fallback', 'multi_turn_context', 'human_handoff'];
}

export function RecognizeScenariosDialog({
  agentId,
  defaultContext,
  open,
  onOpenChange,
  mode = 'conversation',
}: RecognizeScenariosDialogProps) {
  const t = useT('agents.workflowTest.dialogs.recognizeScenarios');
  const commonT = useT('agents.workflowTest.common');
  const createRecognitionTask = useCreateWorkflowTestScenarioRecognitionTask(agentId);
  const { value: defaultModel } = useDefaultModelByUseCase('text-chat');
  const [selectedModel, setSelectedModel] = React.useState<ModelSelectorValue | null>(null);
  const [modelOverridden, setModelOverridden] = React.useState(false);
  React.useEffect(() => {
    if (!open || !defaultModel) return;
    setSelectedModel({ provider: defaultModel.provider, model: defaultModel.model });
    setModelOverridden(false);
  }, [defaultModel, open]);

  const [focusValues, setFocusValues] = React.useState<string[]>(() => defaultFocusValues(mode));
  const [granularity, setGranularity] = React.useState('balanced');
  const [businessPrompt, setBusinessPrompt] = React.useState('');
  const [expertPrompt, setExpertPrompt] = React.useState('');
  const [expertPromptOpen, setExpertPromptOpen] = React.useState(false);

  React.useEffect(() => {
    if (!open) return;
    setFocusValues(defaultFocusValues(mode));
    setGranularity('balanced');
    setBusinessPrompt('');
    setExpertPrompt('');
    setExpertPromptOpen(false);
  }, [mode, open]);

  const canSubmit = Boolean(selectedModel?.provider && selectedModel?.model);

  const toggleFocus = (value: string, checked: boolean) => {
    setFocusValues(prev =>
      checked ? Array.from(new Set([...prev, value])) : prev.filter(item => item !== value)
    );
  };

  const buildPrompt = () => {
    const parts: string[] = [];
    const selectedFocusLabels = FOCUS_OPTIONS.filter(
      item => item.modes.includes(mode) && focusValues.includes(item.value)
    ).map(item => t(item.payloadKey));
    if (selectedFocusLabels.length > 0) {
      parts.push(`${t('focusPayloadTitle')}\n${selectedFocusLabels.join(t('listSeparator'))}`);
    }
    const granularityLabel = t(
      GRANULARITY_OPTIONS.find(item => item.value === granularity)?.payloadKey ??
        'granularityBalanced'
    );
    parts.push(`${t('granularityPayloadTitle')}\n${granularityLabel}`);
    if (businessPrompt.trim()) {
      parts.push(`${t('businessPromptPayloadTitle')}\n${businessPrompt.trim()}`);
    }
    if (expertPrompt.trim()) {
      parts.push(`${t('expertPromptPayloadTitle')}\n${expertPrompt.trim()}`);
    }
    return parts.join('\n\n');
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        size="lg"
        className="max-w-[760px] rounded-2xl"
        onInteractOutside={event => event.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>{t('title')}</DialogTitle>
          <DialogDescription>{t('description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-5">
          <div className="space-y-2">
            <Label>{t('modelLabel')}</Label>
            <ModelSelector
              modelType="text-chat"
              value={selectedModel ?? undefined}
              onChange={value => {
                setSelectedModel(value);
                setModelOverridden(true);
              }}
              placeholder={t('modelPlaceholder')}
            />
          </div>

          <section className="space-y-3">
            <Label>{t('focusLabel')}</Label>
            <div className="flex flex-wrap gap-2">
              {FOCUS_OPTIONS.filter(item => item.modes.includes(mode)).map(item => {
                const checked = focusValues.includes(item.value);
                return (
                  <label
                    key={item.value}
                    className="flex cursor-pointer items-center gap-2 rounded-xl border px-4 py-3 text-sm transition-colors"
                  >
                    <Checkbox
                      checked={checked}
                      onCheckedChange={next => toggleFocus(item.value, Boolean(next))}
                    />
                    <span>{t(item.labelKey)}</span>
                  </label>
                );
              })}
            </div>
          </section>

          <section className="space-y-3">
            <Label>{t('granularityLabel')}</Label>
            <div className="grid grid-cols-3 gap-3">
              {GRANULARITY_OPTIONS.map(item => (
                <Button
                  key={item.value}
                  type="button"
                  variant={granularity === item.value ? 'default' : 'outline'}
                  className="h-12"
                  onClick={() => setGranularity(item.value)}
                >
                  {t(item.labelKey)}
                </Button>
              ))}
            </div>
          </section>

          <div className="space-y-2">
            <Label htmlFor="workflow-test-recognize-business-prompt">
              {t('businessPromptLabel')}
            </Label>
            <Textarea
              id="workflow-test-recognize-business-prompt"
              value={businessPrompt}
              onChange={event => setBusinessPrompt(event.target.value)}
              placeholder={
                mode === 'task'
                  ? t('taskBusinessPromptPlaceholder')
                  : t('businessPromptPlaceholder')
              }
              className="min-h-24 resize-none leading-7"
            />
          </div>

          <section className="space-y-3 rounded-xl border border-slate-200 bg-white p-4">
            <button
              type="button"
              className="flex w-full items-center justify-between text-left"
              onClick={() => setExpertPromptOpen(prev => !prev)}
            >
              <span>
                <span className="block text-sm font-medium text-slate-900">
                  {t('expertPromptLabel')}
                </span>
                <span className="mt-1 block text-sm text-slate-500">
                  {t('expertPromptDescription')}
                </span>
              </span>
              <span className="text-sm font-medium text-blue-600">
                {expertPromptOpen ? t('collapseExpertPrompt') : t('expandExpertPrompt')}
              </span>
            </button>
            {expertPromptOpen ? (
              <Textarea
                value={expertPrompt}
                onChange={event => setExpertPrompt(event.target.value)}
                placeholder={
                  mode === 'task' ? t('taskExpertPromptPlaceholder') : t('expertPromptPlaceholder')
                }
                className="min-h-28 resize-none leading-7"
              />
            ) : null}
          </section>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {commonT('cancel')}
          </Button>
          <Button
            disabled={createRecognitionTask.isPending || !canSubmit}
            onClick={() => {
              if (!selectedModel) return;
              createRecognitionTask.mutate(
                {
                  context: defaultContext,
                  prompt: buildPrompt(),
                  case_mode: mode,
                  model: modelOverridden
                    ? {
                        provider: selectedModel.provider,
                        name: selectedModel.model,
                      }
                    : undefined,
                },
                { onSuccess: () => onOpenChange(false) }
              );
            }}
          >
            {createRecognitionTask.isPending ? t('submitting') : t('submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
