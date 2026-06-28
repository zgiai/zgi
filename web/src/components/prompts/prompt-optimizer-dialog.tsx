'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import { Copy, FlaskConical, WandSparkles } from 'lucide-react';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { ModelSelector, type ModelSelectorValue } from '@/components/common/model-selector';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { useDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { promptService } from '@/services/prompt.service';
import { PromptPlaygroundDialog } from './prompt-playground-dialog';
import type {
  PromptOptimizeResult,
  PromptOptimizerGoal,
  PromptOptimizerVariant,
} from '@/services/types/prompt';
import { extractPromptVariables } from './prompt-optimizer-template';
import { stripZGISlotBlocksForPromptOptimization } from '@/components/workflow/common/workflow-value-editor/utils/value-transform';
import { getPromptRuntimeErrorMessage } from './prompt-runtime-errors';
import { useWorkspaceStore } from '@/store/workspace-store';
import { useLocale } from '@/hooks/use-locale';

interface PromptOptimizerDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialPrompt?: string;
  sourceLabel?: string;
  sourceHelpText?: string;
  sourceResetLabel?: string;
  promptId?: string;
  initialGoal?: PromptOptimizerGoal;
  initialPreserveVariables?: boolean;
  initialModel?: ModelSelectorValue | null;
  initialEditInstruction?: string;
  targetMaxChars?: number;
  lockModel?: boolean;
  onApplyResult?: (payload: {
    text: string;
    variant: PromptOptimizerVariant;
  }) => Promise<unknown> | unknown;
  applyLabel?: string;
}

const optimizerGoals: PromptOptimizerGoal[] = ['general', 'reliable', 'deep'];
const optimizerProgressSteps = ['analyze', 'variables', 'rewrite', 'polish'] as const;
type OptimizerRunStatus = 'idle' | 'streaming' | 'complete' | 'truncated' | 'error';

export function PromptOptimizerDialog({
  open,
  onOpenChange,
  initialPrompt,
  sourceLabel,
  sourceHelpText,
  sourceResetLabel,
  promptId,
  initialGoal,
  initialPreserveVariables,
  initialModel,
  initialEditInstruction,
  targetMaxChars,
  lockModel = false,
  onApplyResult,
  applyLabel,
}: PromptOptimizerDialogProps) {
  const t = useT('prompts');
  const { locale } = useLocale();
  const organizationRole = useWorkspaceStore.use.permissionState().organizationRole;
  const isAdminOrOwner = organizationRole === 'owner' || organizationRole === 'admin';
  const [sourcePrompt, setSourcePrompt] = useState('');
  const [goal, setGoal] = useState<PromptOptimizerGoal>('general');
  const [preserveVariables, setPreserveVariables] = useState(true);
  const [editInstruction, setEditInstruction] = useState('');
  const [selectedModel, setSelectedModel] = useState<ModelSelectorValue | null>(null);
  const [result, setResult] = useState<PromptOptimizeResult | null>(null);
  const [isApplying, setIsApplying] = useState(false);
  const [playgroundOpen, setPlaygroundOpen] = useState(false);
  const [progressStepIndex, setProgressStepIndex] = useState(0);
  const [isStreaming, setIsStreaming] = useState(false);
  const [runStatus, setRunStatus] = useState<OptimizerRunStatus>('idle');
  const [streamOutput, setStreamOutput] = useState('');
  const [streamVariables, setStreamVariables] = useState<string[]>([]);
  const streamHandleRef = useRef<{ close: () => void } | null>(null);
  const { value: defaultModel, source: defaultModelSource } = useDefaultModelByUseCase('text-chat');

  useEffect(() => {
    if (!open) return;
    setSourcePrompt(stripZGISlotBlocksForPromptOptimization(initialPrompt ?? ''));
    setGoal(initialGoal ?? 'general');
    setPreserveVariables(initialPreserveVariables ?? true);
    setEditInstruction(initialEditInstruction ?? '');
    setSelectedModel(initialModel ?? null);
    setResult(null);
    setProgressStepIndex(0);
    setIsStreaming(false);
    setRunStatus('idle');
    setStreamOutput('');
    setStreamVariables([]);
    setPlaygroundOpen(false);
    streamHandleRef.current?.close();
    streamHandleRef.current = null;
  }, [initialEditInstruction, initialGoal, initialModel, initialPreserveVariables, initialPrompt, open]);

  useEffect(() => {
    return () => {
      streamHandleRef.current?.close();
      streamHandleRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (
      !open ||
      selectedModel ||
      initialModel?.model ||
      !defaultModel?.provider ||
      !defaultModel?.model
    ) {
      return;
    }
    setSelectedModel({
      provider: defaultModel.provider,
      model: defaultModel.model,
    });
  }, [defaultModel?.model, defaultModel?.provider, initialModel?.model, open, selectedModel]);

  const detectedVariables = useMemo(() => extractPromptVariables(sourcePrompt), [sourcePrompt]);
  const visibleVariables =
    result?.detected_variables?.length
      ? result.detected_variables
      : streamVariables.length > 0
        ? streamVariables
        : detectedVariables;
  const currentOutput = result?.output ?? streamOutput;
  const canApplyResult = Boolean(onApplyResult && result?.output && !result.truncated && runStatus === 'complete');
  const canCopyResult = Boolean(result?.output);
  const canTestResult = Boolean(result?.output && !result.truncated);
  const activeModel = selectedModel ?? (defaultModel?.provider && defaultModel?.model
    ? {
        provider: defaultModel.provider,
        model: defaultModel.model,
      }
    : null);

  const handleRun = async (nextGoal: PromptOptimizerGoal = goal) => {
    if (!sourcePrompt.trim()) return;
    setGoal(nextGoal);
    setResult(null);
    setStreamOutput('');
    setStreamVariables([]);
    setProgressStepIndex(0);
    setIsStreaming(true);
    setRunStatus('streaming');
    streamHandleRef.current?.close();
    streamHandleRef.current = null;

    try {
      streamHandleRef.current = await promptService.streamOptimizePrompt(
        {
          raw_prompt: sourcePrompt,
          goal: nextGoal,
          preserve_variables: preserveVariables,
          provider: activeModel?.provider,
          model: activeModel?.model,
          prompt_id: promptId,
          language: locale,
          edit_instruction: editInstruction.trim() || undefined,
          target_max_chars: targetMaxChars,
        },
        {
          onProgress: payload => {
            const step = payload.step;
            const nextIndex = optimizerProgressSteps.findIndex(item => item === step);
            if (nextIndex >= 0) {
              setProgressStepIndex(nextIndex);
            }
          },
          onMeta: payload => {
            setStreamVariables(payload.detected_variables ?? []);
          },
          onChunk: payload => {
            if (typeof payload.text !== 'string' || payload.text.length === 0) {
              return;
            }
            setStreamOutput(previous => `${previous}${payload.text ?? ''}`);
          },
          onDone: payload => {
            const output = typeof payload.output === 'string' ? payload.output : '';
            const truncated = Boolean(payload.truncated);
            setProgressStepIndex(optimizerProgressSteps.length - 1);
            setResult({
              goal: nextGoal,
              preserve_variables: preserveVariables,
              detected_variables: payload.detected_variables ?? [],
              run_id: payload.run_id ?? '',
              output,
              truncated,
              finish_reason: payload.finish_reason,
              target_max_chars: payload.target_max_chars,
              variants: {
                safe: output,
                balanced: output,
                advanced: output,
              },
            });
            setStreamOutput(output);
            setStreamVariables(payload.detected_variables ?? []);
            setIsStreaming(false);
            setRunStatus(truncated ? 'truncated' : 'complete');
            streamHandleRef.current = null;
          },
          onError: error => {
            const modelLabel = selectedModel?.model
              ? selectedModel.provider
                ? `${selectedModel.provider} / ${selectedModel.model}`
                : selectedModel.model
              : undefined;
            const normalized = getPromptRuntimeErrorMessage(error, modelLabel, isAdminOrOwner, (key, values) =>
              t(key as never, values as never)
            );
            toast.error(normalized.message);
            setIsStreaming(false);
            setRunStatus('error');
            streamHandleRef.current = null;
          },
          onClose: () => {
            setIsStreaming(false);
            streamHandleRef.current = null;
          },
        }
      );
    } catch (error) {
      const modelLabel = selectedModel?.model
        ? selectedModel.provider
          ? `${selectedModel.provider} / ${selectedModel.model}`
          : selectedModel.model
        : undefined;
      const normalized = getPromptRuntimeErrorMessage(error, modelLabel, isAdminOrOwner, (key, values) =>
        t(key as never, values as never)
      );
      toast.error(normalized.message);
      setIsStreaming(false);
      setRunStatus('error');
    }
  };

  const handleSourceChange = (value: string) => {
    setSourcePrompt(value);
    setResult(null);
    setStreamOutput('');
    setRunStatus('idle');
  };

  const handleGoalChange = (value: PromptOptimizerGoal) => {
    setGoal(value);
    setResult(null);
    setStreamOutput('');
    setRunStatus('idle');
  };

  const handlePreserveVariablesChange = (checked: boolean) => {
    setPreserveVariables(checked);
    setResult(null);
    setStreamOutput('');
    setRunStatus('idle');
  };

  const handleEditInstructionChange = (value: string) => {
    setEditInstruction(value);
    setResult(null);
    setStreamOutput('');
    setRunStatus('idle');
  };

  const handleCopyCurrent = async () => {
    if (!currentOutput) return;
    try {
      await navigator.clipboard.writeText(currentOutput);
      toast.success(t('messages.optimizerCopied'));
    } catch {
      toast.error(t('messages.optimizerCopyFailed'));
    }
  };

  const handleApply = async () => {
    if (!canApplyResult || !result?.output || !onApplyResult) return;
    setIsApplying(true);
    try {
      await onApplyResult({
        text: result.output,
        variant: 'balanced',
      });
      onOpenChange(false);
    } finally {
      setIsApplying(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="h-[min(calc(100vh-48px),820px)] w-[min(calc(100vw-48px),1280px)] max-w-none">
        <DialogHeader className="border-b px-6 py-5">
          <DialogTitle>{t('optimizer.title')}</DialogTitle>
          <div className="max-w-3xl text-sm text-muted-foreground">{t('optimizer.description')}</div>
        </DialogHeader>
        <DialogBody className="grid min-h-0 flex-1 grid-rows-[minmax(0,1fr)_minmax(0,1fr)] gap-0 overflow-hidden p-0 md:grid-cols-[400px_minmax(0,1fr)] md:grid-rows-none xl:grid-cols-[420px_minmax(0,1fr)]">
          <section className="flex min-h-0 flex-col overflow-hidden border-b bg-muted/10 md:border-b-0 md:border-r">
            <div className="border-b bg-background px-5 py-4">
              <div className="text-sm font-medium">{t('optimizer.inputPanelLabel')}</div>
              <div className="mt-1 text-xs text-muted-foreground">{t('optimizer.inputPanelDescription')}</div>
            </div>

            <div className="min-h-0 flex-1 space-y-5 overflow-y-auto px-5 py-4">
              <div className="space-y-2">
                <div className="flex items-center justify-between gap-3">
                  <Label>{sourceLabel || t('optimizer.sourceLabel')}</Label>
                  {initialPrompt ? (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => handleSourceChange(initialPrompt)}
                      disabled={sourcePrompt === initialPrompt}
                    >
                      {sourceResetLabel || t('optimizer.resetSource')}
                    </Button>
                  ) : null}
                </div>
                <div className="text-xs text-muted-foreground">
                  {sourceHelpText ||
                    (initialPrompt
                      ? t('optimizer.prefilledSourceDescription')
                      : t('optimizer.sourceHelpDescription'))}
                </div>
                <Textarea
                  value={sourcePrompt}
                  onChange={e => handleSourceChange(e.target.value)}
                  placeholder={t('optimizer.sourcePlaceholder')}
                  className="min-h-40 resize-none"
                />
              </div>

              <div className="space-y-2">
                <Label>{t('optimizer.editInstructionLabel')}</Label>
                <div className="text-xs text-muted-foreground">
                  {t('optimizer.editInstructionDescription')}
                </div>
                <Textarea
                  value={editInstruction}
                  onChange={e => handleEditInstructionChange(e.target.value)}
                  placeholder={t('optimizer.editInstructionPlaceholder')}
                  className="min-h-24 resize-none"
                  maxLength={2000}
                />
              </div>

              <div className="space-y-4 rounded-lg border bg-background p-4">
                <div>
                  <div className="text-sm font-medium">{t('optimizer.settingsPanelLabel')}</div>
                  <div className="mt-1 text-xs text-muted-foreground">{t('optimizer.settingsPanelDescription')}</div>
                </div>

                <div className="space-y-2">
                  <Label>{t('optimizer.modelLabel')}</Label>
                  {lockModel ? (
                    <div className="rounded-lg border bg-muted/20 px-3 py-3 text-sm">
                      <div className="font-medium">
                        {activeModel?.model
                          ? activeModel.provider
                            ? `${activeModel.provider} / ${activeModel.model}`
                            : activeModel.model
                          : t('optimizer.modelUnavailable')}
                      </div>
                      <div className="mt-1 text-xs text-muted-foreground">
                        {t('optimizer.fixedModelDescription')}
                      </div>
                    </div>
                  ) : (
                    <>
                      <ModelSelector
                        modelType="text-chat"
                        value={selectedModel ?? undefined}
                        onChange={value => {
                          setSelectedModel(value);
                          setResult(null);
                          setStreamOutput('');
                          setRunStatus('idle');
                        }}
                      />
                      <div className="text-xs text-muted-foreground">
                        {t('optimizer.modelDescription')}
                        {selectedModel?.provider && selectedModel?.model
                          ? ` ${selectedModel.provider} / ${selectedModel.model}`
                          : ''}
                        {defaultModelSource !== 'none' && !selectedModel?.provider && defaultModel?.provider
                          ? ` (${defaultModel.provider} / ${defaultModel.model})`
                          : ''}
                      </div>
                    </>
                  )}
                </div>

                <div className="space-y-2">
                  <Label>{t('optimizer.goalLabel')}</Label>
                  <div className="flex flex-wrap gap-2">
                    {optimizerGoals.map(goalOption => (
                      <Button
                        key={goalOption}
                        type="button"
                        size="sm"
                        variant={goal === goalOption ? 'default' : 'outline'}
                        onClick={() => handleGoalChange(goalOption)}
                        disabled={isStreaming}
                      >
                        <WandSparkles className="h-4 w-4" />
                        {t(`optimizer.goals.${goalOption}.label`)}
                      </Button>
                    ))}
                  </div>
                  <div className="text-xs text-muted-foreground">{t(`optimizer.goals.${goal}.description`)}</div>
                </div>

                {targetMaxChars ? (
                  <div className="rounded-md bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
                    {t('optimizer.targetMaxChars', { count: targetMaxChars })}
                  </div>
                ) : null}

                <div className="space-y-3">
                  <div className="flex items-center justify-between gap-3">
                    <div className="space-y-1">
                      <Label>{t('optimizer.preserveVariablesLabel')}</Label>
                      <div className="text-xs text-muted-foreground">
                        {t('optimizer.preserveVariablesDescription')}
                      </div>
                    </div>
                    <Switch
                      checked={preserveVariables}
                      onCheckedChange={checked => handlePreserveVariablesChange(Boolean(checked))}
                    />
                  </div>
                  <div className="min-w-0 rounded-lg bg-muted/30 p-3 space-y-2">
                    <div className="text-xs font-medium text-muted-foreground">
                      {t('optimizer.detectedVariablesLabel')}
                    </div>
                    {visibleVariables.length > 0 ? (
                      <div className="flex min-w-0 max-w-full flex-wrap gap-2">
                        {visibleVariables.map(variable => (
                          <Badge
                            key={variable}
                            variant="outline"
                            className="max-w-full whitespace-normal break-all text-left leading-5"
                          >
                            {variable}
                          </Badge>
                        ))}
                      </div>
                    ) : (
                      <div className="text-xs text-muted-foreground">{t('optimizer.noVariables')}</div>
                    )}
                  </div>
                </div>
              </div>
            </div>

            <div className="border-t bg-background px-5 py-4">
              <Button
                type="button"
                className="w-full"
                onClick={() => void handleRun()}
                disabled={isStreaming || !sourcePrompt.trim()}
                loading={isStreaming}
              >
                <WandSparkles className="h-4 w-4" />
                {result ? t('optimizer.rerun') : t('optimizer.run')}
              </Button>
            </div>
          </section>

          <section className="flex min-h-0 flex-col overflow-hidden bg-background">
            <div className="border-b px-6 py-4">
              <div className="space-y-1">
                <Label>{t('optimizer.outputLabel')}</Label>
                <div className="text-xs text-muted-foreground">{t('optimizer.variantHint')}</div>
              </div>
            </div>

            <div className="min-h-0 flex-1 overflow-y-auto p-6">
              {isStreaming ? (
                <div className="space-y-4">
                  <div className="rounded-lg border bg-muted/20 p-4 space-y-3">
                    <div className="text-sm font-medium">{t('optimizer.progress.title')}</div>
                    <div className="grid gap-2 sm:grid-cols-4">
                      {optimizerProgressSteps.map((step, index) => {
                        const isDone = index < progressStepIndex;
                        const isCurrent = index === progressStepIndex;
                        return (
                          <div
                            key={step}
                            className={`rounded-md border px-3 py-2 text-sm ${
                              isCurrent
                                ? 'border-primary bg-primary/5 text-foreground'
                                : isDone
                                  ? 'border-border bg-background text-foreground'
                                  : 'border-border/60 text-muted-foreground'
                            }`}
                          >
                            {t(`optimizer.progress.steps.${step}`)}
                          </div>
                        );
                      })}
                    </div>
                    <div className="text-sm text-muted-foreground">{t('optimizer.runningDescription')}</div>
                  </div>
                  <Textarea value={streamOutput} readOnly className="min-h-[420px] resize-none font-mono text-xs" />
                </div>
              ) : !result ? (
                <div className="flex min-h-[320px] items-center justify-center rounded-lg bg-muted/20 px-8 text-center">
                  <div className="max-w-sm space-y-2">
                    <div className="text-sm font-medium text-foreground">
                      {runStatus === 'error' ? t('optimizer.errorStateTitle') : t('optimizer.waitingTitle')}
                    </div>
                    <div className="text-sm text-muted-foreground">
                      {runStatus === 'error' ? t('optimizer.errorStateDescription') : t('optimizer.emptyState')}
                    </div>
                  </div>
                </div>
              ) : (
                <div className="space-y-3">
                  <div className="rounded-lg border bg-muted/20 p-3 text-sm text-muted-foreground">
                    {t(`optimizer.goals.${goal}.description`)}
                  </div>
                  {result.truncated ? (
                    <div className="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
                      {t('optimizer.truncatedWarning')}
                    </div>
                  ) : null}
                  <Textarea value={currentOutput} readOnly className="min-h-[460px] resize-none font-mono text-xs" />
                </div>
              )}
            </div>

            {result ? (
              <div className="flex flex-wrap items-center justify-end gap-2 border-t px-6 py-4">
                <Button
                  variant="outline"
                  onClick={() => setPlaygroundOpen(true)}
                  disabled={!canTestResult}
                >
                  <FlaskConical className="h-4 w-4" />
                  {t('actions.testInPlayground')}
                </Button>
                <Button variant="outline" onClick={handleCopyCurrent} disabled={!canCopyResult}>
                  <Copy className="h-4 w-4" />
                  {result?.truncated ? t('optimizer.copyPartial') : t('optimizer.copy')}
                </Button>
                {onApplyResult ? (
                  <Button onClick={handleApply} disabled={!canApplyResult} loading={isApplying}>
                    <WandSparkles className="h-4 w-4" />
                    {applyLabel || t('optimizer.apply')}
                  </Button>
                ) : null}
              </div>
            ) : null}
          </section>
        </DialogBody>
        <DialogFooter className="border-t px-6 py-4">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel')}
          </Button>
        </DialogFooter>
      </DialogContent>
      <PromptPlaygroundDialog
        open={playgroundOpen}
        onOpenChange={setPlaygroundOpen}
        prefillPromptText={currentOutput}
        prefillPromptLabel={t('optimizer.outputLabel')}
      />
    </Dialog>
  );
}

export default PromptOptimizerDialog;
