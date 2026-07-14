'use client';

import * as React from 'react';
import { Info } from 'lucide-react';
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
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Checkbox } from '@/components/ui/checkbox';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { ModelSelector } from '@/components/common/model-selector';
import type { ModelSelectorValue } from '@/components/common/model-selector';
import { useCreateWorkflowTestGenerationTask } from '@/hooks/workflow-test/use-workflow-test';
import { useDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { useCurrentUser } from '@/store/auth-store';
import { getLastSelectedAiModel, saveLastSelectedAiModel } from '@/utils/ui-local';
import { useT } from '@/i18n';
import {
  DEFAULT_QUESTION_TYPES,
  DEFAULT_TASK_QUESTION_TYPES,
  QUESTION_TYPE_OPTIONS,
} from './question-type';
import type { WorkflowTestMode } from './case-metadata';

interface GenerateCasesDialogProps {
  agentId: string;
  scenarios: Array<{ id: string; name: string }>;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onGenerationStart?: (count: number) => void;
  onGenerationCreateFailed?: () => void;
  mode?: WorkflowTestMode;
  supportsGeneratedFiles?: boolean;
  requiresCurrentTurnFiles?: boolean;
}

const MIN_GENERATED_CASE_COUNT = 1;
const MAX_GENERATED_CASE_COUNT = 50;
const COUNT_PRESETS = [10, 20, MAX_GENERATED_CASE_COUNT];
const TURN_STRATEGIES = [
  { value: 'mixed' as const, labelKey: 'turnStrategyMixed' as const },
  { value: 'single' as const, labelKey: 'turnStrategySingle' as const },
  { value: 'multi' as const, labelKey: 'turnStrategyMulti' as const },
];
const FILE_FORMAT_OPTIONS = [
  { value: 'docx', labelKey: 'fileFormatDocx' as const },
  { value: 'pdf', labelKey: 'fileFormatPdf' as const },
  { value: 'txt', labelKey: 'fileFormatTxt' as const },
  { value: 'csv', labelKey: 'fileFormatCsv' as const },
  { value: 'xlsx', labelKey: 'fileFormatXlsx' as const },
];
const FILE_COMPLEXITY_OPTIONS = [
  { value: 'normal', labelKey: 'fileComplexityNormal' as const },
  { value: 'noisy', labelKey: 'fileComplexityNoisy' as const },
  { value: 'missing_fields', labelKey: 'fileComplexityMissingFields' as const },
];
const FILES_PER_CASE_PRESETS = [1, 2, 3];

export function GenerateCasesDialog({
  agentId,
  scenarios,
  open,
  onOpenChange,
  onGenerationStart,
  onGenerationCreateFailed,
  mode = 'conversation',
  supportsGeneratedFiles = false,
  requiresCurrentTurnFiles = false,
}: GenerateCasesDialogProps) {
  const t = useT('agents.workflowTest.dialogs.generateCases');
  const commonT = useT('agents.workflowTest.common');
  const typeT = useT('agents.workflowTest.questionTypes');
  const createGenerationTask = useCreateWorkflowTestGenerationTask(agentId);
  const user = useCurrentUser();
  const { value: defaultModel } = useDefaultModelByUseCase('text-chat');
  const [count, setCount] = React.useState(20);
  const [scenarioIds, setScenarioIds] = React.useState<string[]>([]);
  const [questionTypes, setQuestionTypes] = React.useState<string[]>(DEFAULT_QUESTION_TYPES);
  const [turnStrategy, setTurnStrategy] = React.useState<'mixed' | 'single' | 'multi'>('mixed');
  const [model, setModel] = React.useState<ModelSelectorValue | null>(null);
  const [businessPrompt, setBusinessPrompt] = React.useState('');
  const [expertPrompt, setExpertPrompt] = React.useState('');
  const [expertPromptOpen, setExpertPromptOpen] = React.useState(false);
  const [generateFiles, setGenerateFiles] = React.useState(false);
  const [fileFormats, setFileFormats] = React.useState<string[]>(['docx', 'pdf']);
  const [filesPerCase, setFilesPerCase] = React.useState(1);
  const [fileComplexities, setFileComplexities] = React.useState<string[]>(['normal']);
  const scenarioSelectionTouchedRef = React.useRef(false);
  const canGenerateFiles = supportsGeneratedFiles;
  const effectiveGenerateFiles = canGenerateFiles && generateFiles;
  const forceSingleFileTurn =
    mode === 'conversation' && effectiveGenerateFiles && requiresCurrentTurnFiles;
  const defaultQuestionTypes =
    mode === 'task' ? DEFAULT_TASK_QUESTION_TYPES : DEFAULT_QUESTION_TYPES;
  const titleLabel = effectiveGenerateFiles
    ? t('fileTitle')
    : mode === 'task'
      ? t('taskTitle')
      : t('title');
  const submitLabel = effectiveGenerateFiles
    ? t('submitFile')
    : mode === 'task'
      ? t('submitTask')
      : t('submit');

  React.useEffect(() => {
    if (!user?.id) return;
    const saved = getLastSelectedAiModel(user.id, 'workflowTestScenario');
    if (saved) {
      setModel({ provider: saved.provider, model: saved.model });
      return;
    }
    if (defaultModel) {
      setModel({ provider: defaultModel.provider, model: defaultModel.model });
    }
  }, [defaultModel, user?.id]);

  React.useEffect(() => {
    if (open) return;
    setCount(20);
    setScenarioIds([]);
    setQuestionTypes(defaultQuestionTypes);
    setTurnStrategy('mixed');
    setBusinessPrompt('');
    setExpertPrompt('');
    setExpertPromptOpen(false);
    setGenerateFiles(false);
    setFileFormats(['docx', 'pdf']);
    setFilesPerCase(1);
    setFileComplexities(['normal']);
    scenarioSelectionTouchedRef.current = false;
  }, [defaultQuestionTypes, open]);

  React.useEffect(() => {
    if (!open) return;
    setQuestionTypes(prev => {
      if (
        prev.length > 0 &&
        prev.every(item => QUESTION_TYPE_OPTIONS.some(option => option.value === item))
      ) {
        return prev;
      }
      return defaultQuestionTypes;
    });
  }, [defaultQuestionTypes, open]);

  React.useEffect(() => {
    if (forceSingleFileTurn) setTurnStrategy('single');
  }, [forceSingleFileTurn]);

  React.useEffect(() => {
    if (!open) return;
    if (scenarioSelectionTouchedRef.current || scenarios.length === 0) return;
    setScenarioIds(prev => {
      if (prev.length > 0) return prev;
      return scenarios.map(scene => scene.id);
    });
  }, [mode, open, scenarios, t]);

  React.useEffect(() => {
    if (!open) return;
    setGenerateFiles(canGenerateFiles);
  }, [canGenerateFiles, open]);

  const safeCount = Number.isFinite(count) ? count : 0;
  const canSubmit =
    safeCount >= MIN_GENERATED_CASE_COUNT &&
    safeCount <= MAX_GENERATED_CASE_COUNT &&
    scenarioIds.length > 0 &&
    questionTypes.length > 0 &&
    (!effectiveGenerateFiles || fileFormats.length > 0) &&
    Boolean(model?.provider && model?.model) &&
    !createGenerationTask.isPending;

  const toggleScenario = (id: string, checked: boolean) => {
    scenarioSelectionTouchedRef.current = true;
    setScenarioIds(prev =>
      checked ? Array.from(new Set([...prev, id])) : prev.filter(sceneId => sceneId !== id)
    );
  };

  const selectAllScenarios = () => {
    scenarioSelectionTouchedRef.current = true;
    setScenarioIds(scenarios.map(scene => scene.id));
  };

  const clearScenarios = () => {
    scenarioSelectionTouchedRef.current = true;
    setScenarioIds([]);
  };

  const toggleQuestionType = (value: string, checked: boolean) => {
    setQuestionTypes(prev =>
      checked ? Array.from(new Set([...prev, value])) : prev.filter(item => item !== value)
    );
  };

  const toggleFileFormat = (value: string, checked: boolean) => {
    setFileFormats(prev =>
      checked ? Array.from(new Set([...prev, value])) : prev.filter(item => item !== value)
    );
  };

  const toggleFileComplexity = (value: string, checked: boolean) => {
    setFileComplexities(prev =>
      checked ? Array.from(new Set([...prev, value])) : prev.filter(item => item !== value)
    );
  };

  const buildPrompt = () => {
    const parts: string[] = [];
    const business = businessPrompt.trim();
    const expert = expertPrompt.trim();
    if (business) {
      parts.push(`${t('businessPromptPayloadTitle')}\n${business}`);
    }
    if (expert) {
      parts.push(`${t('expertPromptPayloadTitle')}\n${expert}`);
    }
    return parts.join('\n\n');
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        size="lg"
        className="w-[calc(100vw-32px)] max-w-[800px] rounded-2xl"
        onInteractOutside={event => event.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>{titleLabel}</DialogTitle>
          <DialogDescription>
            {mode === 'task' ? t('taskDescription') : t('description')}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="max-h-[calc(100vh-220px)] space-y-6 overflow-y-auto pr-1">
          <section className="space-y-3">
            <Label>{t('countLabel')}</Label>
            <div className="flex flex-wrap items-center gap-2">
              <div className="w-28">
                <Input
                  type="number"
                  min={MIN_GENERATED_CASE_COUNT}
                  max={MAX_GENERATED_CASE_COUNT}
                  value={count}
                  onChange={event => {
                    const next = Number(event.target.value);
                    if (!Number.isFinite(next)) {
                      setCount(MIN_GENERATED_CASE_COUNT);
                      return;
                    }
                    setCount(
                      Math.min(MAX_GENERATED_CASE_COUNT, Math.max(MIN_GENERATED_CASE_COUNT, next))
                    );
                  }}
                />
              </div>
              {COUNT_PRESETS.map(preset => (
                <Button
                  key={preset}
                  type="button"
                  variant={count === preset ? 'default' : 'outline'}
                  size="sm"
                  onClick={() => setCount(preset)}
                >
                  {preset}
                </Button>
              ))}
            </div>
          </section>

          <section className="space-y-3">
            <div className="flex items-center justify-between">
              <Label>{t('scenarioLabel')}</Label>
              <div className="flex items-center gap-3 text-sm text-slate-500">
                <button type="button" className="font-medium" onClick={selectAllScenarios}>
                  {commonT('selectAll')}
                </button>
                <button type="button" className="font-medium" onClick={clearScenarios}>
                  {commonT('clearSelection')}
                </button>
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              {scenarios.map(scene => {
                const checked = scenarioIds.includes(scene.id);
                return (
                  <label
                    key={scene.id}
                    className="flex cursor-pointer items-center gap-2 rounded-xl border px-4 py-3 text-sm transition-colors"
                  >
                    <Checkbox
                      checked={checked}
                      onCheckedChange={next => toggleScenario(scene.id, Boolean(next))}
                    />
                    <span>{scene.name}</span>
                  </label>
                );
              })}
            </div>
            {mode === 'task' ? (
              <p className="text-sm text-slate-500">{t('taskDimensionDescription')}</p>
            ) : null}
          </section>

          <section className="space-y-3">
            <Label>{t('questionTypeLabel')}</Label>
            <p className="text-sm text-slate-500">
              {mode === 'task' ? t('taskQuestionTypeDescription') : t('questionTypeDescription')}
            </p>
            <div className="flex flex-wrap gap-2">
              {QUESTION_TYPE_OPTIONS.map(item => {
                const checked = questionTypes.includes(item.value);
                const label =
                  mode === 'task'
                    ? t(
                        item.value === 'core'
                          ? 'taskQuestionTypeCore'
                          : item.value === 'extension'
                            ? 'taskQuestionTypeExtension'
                            : item.value === 'fuzzy'
                              ? 'taskQuestionTypeFuzzy'
                              : 'taskQuestionTypeManual'
                      )
                    : typeT(item.labelKey);
                return (
                  <label
                    key={item.value}
                    className="flex cursor-pointer items-center gap-2 rounded-xl border px-4 py-3 text-sm transition-colors"
                  >
                    <Checkbox
                      checked={checked}
                      onCheckedChange={next => toggleQuestionType(item.value, Boolean(next))}
                    />
                    <span>{label}</span>
                  </label>
                );
              })}
            </div>
          </section>

          {mode === 'conversation' ? (
            <section className="space-y-3">
              <div className="flex items-center gap-2">
                <Label>{t('turnStrategyLabel')}</Label>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <button
                      type="button"
                      className="inline-flex size-5 items-center justify-center rounded-full text-slate-400 transition-colors hover:text-slate-700"
                      aria-label={t('turnStrategyHelpLabel')}
                    >
                      <Info className="size-4" />
                    </button>
                  </TooltipTrigger>
                  <TooltipContent
                    side="top"
                    align="start"
                    className="max-w-[calc(100vw-48px)] space-y-1 text-sm leading-6"
                  >
                    <p className="whitespace-nowrap">{t('turnStrategyHelpMixed')}</p>
                    <p className="whitespace-nowrap">{t('turnStrategyHelpSingle')}</p>
                    <p className="whitespace-nowrap">{t('turnStrategyHelpMulti')}</p>
                  </TooltipContent>
                </Tooltip>
              </div>
              <div className="grid grid-cols-3 gap-3">
                {TURN_STRATEGIES.map(item => (
                  <Button
                    key={item.value}
                    type="button"
                    variant={turnStrategy === item.value ? 'default' : 'outline'}
                    className="h-12"
                    disabled={forceSingleFileTurn && item.value !== 'single'}
                    onClick={() => setTurnStrategy(item.value)}
                  >
                    {t(item.labelKey)}
                  </Button>
                ))}
              </div>
              {forceSingleFileTurn ? (
                <p className="text-sm text-amber-700">{t('turnStrategyFilesRequired')}</p>
              ) : null}
            </section>
          ) : null}

          {canGenerateFiles ? (
            <section className="space-y-4 rounded-xl border border-slate-200 bg-slate-50 p-4">
              <div className="flex items-start justify-between gap-4">
                <div>
                  <Label>{t('fileGenerationLabel')}</Label>
                  <p className="mt-1 text-sm text-slate-500">
                    {mode === 'conversation'
                      ? t('fileGenerationConversationDescription')
                      : t('fileGenerationDescription')}
                  </p>
                </div>
                <Checkbox
                  checked={generateFiles}
                  onCheckedChange={next => setGenerateFiles(Boolean(next))}
                />
              </div>
              {generateFiles ? (
                <>
                  <div className="space-y-3">
                    <Label>{t('fileFormatsLabel')}</Label>
                    <div className="flex flex-wrap gap-2">
                      {FILE_FORMAT_OPTIONS.map(item => {
                        const checked = fileFormats.includes(item.value);
                        return (
                          <label
                            key={item.value}
                            className="flex cursor-pointer items-center gap-2 rounded-xl border bg-white px-4 py-3 text-sm transition-colors"
                          >
                            <Checkbox
                              checked={checked}
                              onCheckedChange={next => toggleFileFormat(item.value, Boolean(next))}
                            />
                            <span>{t(item.labelKey)}</span>
                          </label>
                        );
                      })}
                    </div>
                  </div>
                  <div className="space-y-3">
                    <Label>{t('filesPerCaseLabel')}</Label>
                    <div className="flex flex-wrap gap-2">
                      {FILES_PER_CASE_PRESETS.map(preset => (
                        <Button
                          key={preset}
                          type="button"
                          variant={filesPerCase === preset ? 'default' : 'outline'}
                          size="sm"
                          onClick={() => setFilesPerCase(preset)}
                        >
                          {preset}
                        </Button>
                      ))}
                    </div>
                  </div>
                  <div className="space-y-3">
                    <Label>{t('fileComplexityLabel')}</Label>
                    <div className="flex flex-wrap gap-2">
                      {FILE_COMPLEXITY_OPTIONS.map(item => {
                        const checked = fileComplexities.includes(item.value);
                        return (
                          <label
                            key={item.value}
                            className="flex cursor-pointer items-center gap-2 rounded-xl border bg-white px-4 py-3 text-sm transition-colors"
                          >
                            <Checkbox
                              checked={checked}
                              onCheckedChange={next =>
                                toggleFileComplexity(item.value, Boolean(next))
                              }
                            />
                            <span>{t(item.labelKey)}</span>
                          </label>
                        );
                      })}
                    </div>
                  </div>
                </>
              ) : null}
            </section>
          ) : null}

          <section className="space-y-3">
            <Label>{t('modelLabel')}</Label>
            <ModelSelector
              modelType="text-chat"
              value={model ?? undefined}
              onChange={value => {
                setModel(value);
                if (user?.id) {
                  saveLastSelectedAiModel(user.id, 'workflowTestScenario', {
                    provider: value.provider,
                    model: value.model,
                  });
                }
              }}
              placeholder={t('modelPlaceholder')}
            />
          </section>

          <section className="space-y-3">
            <Label htmlFor="workflow-test-generate-business-prompt">
              {t('businessPromptLabel')}
            </Label>
            <Textarea
              id="workflow-test-generate-business-prompt"
              value={businessPrompt}
              onChange={event => setBusinessPrompt(event.target.value)}
              placeholder={
                mode === 'task'
                  ? t('taskBusinessPromptPlaceholder')
                  : t('businessPromptPlaceholder')
              }
              className="min-h-24 resize-none leading-7"
            />
          </section>

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
            disabled={!canSubmit}
            onClick={() => {
              if (!model) return;
              const payload = {
                count: safeCount,
                scenario_ids: scenarioIds,
                question_types: questionTypes,
                turn_strategy: mode === 'task' || forceSingleFileTurn ? 'single' : turnStrategy,
                case_mode: mode,
                file_generation: effectiveGenerateFiles
                  ? {
                      enabled: true,
                      formats: fileFormats,
                      files_per_case: filesPerCase,
                      complexities: fileComplexities,
                      content_types:
                        mode === 'conversation'
                          ? ['conversation_attachment']
                          : ['workflow_input_document'],
                    }
                  : undefined,
                prompt: buildPrompt(),
                model: {
                  provider: model.provider,
                  name: model.model,
                },
              };
              onGenerationStart?.(safeCount);
              onOpenChange(false);
              createGenerationTask.mutate(payload, { onError: () => onGenerationCreateFailed?.() });
            }}
          >
            {createGenerationTask.isPending ? t('submitting') : submitLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
