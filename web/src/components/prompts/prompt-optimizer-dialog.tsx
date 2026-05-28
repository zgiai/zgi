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
  lockModel?: boolean;
  onApplyResult?: (payload: {
    text: string;
    variant: PromptOptimizerVariant;
  }) => Promise<unknown> | unknown;
  applyLabel?: string;
}

const optimizerGoals: PromptOptimizerGoal[] = ['general', 'reliable', 'deep'];
const optimizerProgressSteps = ['analyze', 'variables', 'rewrite', 'polish'] as const;

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
  const [selectedModel, setSelectedModel] = useState<ModelSelectorValue | null>(null);
  const [result, setResult] = useState<PromptOptimizeResult | null>(null);
  const [isApplying, setIsApplying] = useState(false);
  const [playgroundOpen, setPlaygroundOpen] = useState(false);
  const [progressStepIndex, setProgressStepIndex] = useState(0);
  const [isStreaming, setIsStreaming] = useState(false);
  const [streamOutput, setStreamOutput] = useState('');
  const [streamVariables, setStreamVariables] = useState<string[]>([]);
  const streamHandleRef = useRef<{ close: () => void } | null>(null);
  const { value: defaultModel, source: defaultModelSource } = useDefaultModelByUseCase('text-chat');

  useEffect(() => {
    if (!open) return;
    setSourcePrompt(stripZGISlotBlocksForPromptOptimization(initialPrompt ?? ''));
    setGoal(initialGoal ?? 'general');
    setPreserveVariables(initialPreserveVariables ?? true);
    setSelectedModel(initialModel ?? null);
    setResult(null);
    setProgressStepIndex(0);
    setIsStreaming(false);
    setStreamOutput('');
    setStreamVariables([]);
    setPlaygroundOpen(false);
    streamHandleRef.current?.close();
    streamHandleRef.current = null;
  }, [initialGoal, initialModel, initialPreserveVariables, initialPrompt, open]);

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
            setProgressStepIndex(optimizerProgressSteps.length - 1);
            setResult({
              goal: nextGoal,
              preserve_variables: preserveVariables,
              detected_variables: payload.detected_variables ?? [],
              run_id: payload.run_id ?? '',
              output,
              variants: {
                safe: output,
                balanced: output,
                advanced: output,
              },
            });
            setStreamOutput(output);
            setStreamVariables(payload.detected_variables ?? []);
            setIsStreaming(false);
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
    }
  };

  const handleSourceChange = (value: string) => {
    setSourcePrompt(value);
    setResult(null);
  };

  const handleGoalChange = (value: PromptOptimizerGoal) => {
    void handleRun(value);
  };

  const handlePreserveVariablesChange = (checked: boolean) => {
    setPreserveVariables(checked);
    setResult(null);
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
    if (!onApplyResult || !currentOutput) return;
    setIsApplying(true);
    try {
      await onApplyResult({
        text: currentOutput,
        variant: 'balanced',
      });
      onOpenChange(false);
    } finally {
      setIsApplying(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-6xl">
        <DialogHeader>
          <DialogTitle>{t('optimizer.title')}</DialogTitle>
        </DialogHeader>
        <DialogBody className="grid gap-5 lg:grid-cols-[360px_minmax(0,1fr)]">
          <div className="space-y-5">
            <div className="rounded-lg border bg-muted/20 p-3 text-sm text-muted-foreground">
              {t('optimizer.description')}
            </div>

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
                className="min-h-40"
              />
            </div>

            <div className="rounded-xl border p-4 space-y-4">
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
                      disabled={isStreaming || !sourcePrompt.trim()}
                    >
                      <WandSparkles className="h-4 w-4" />
                      {t(`optimizer.goals.${goalOption}.label`)}
                    </Button>
                  ))}
                </div>
                <div className="text-xs text-muted-foreground">{t(`optimizer.goals.${goal}.description`)}</div>
              </div>

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
                <div className="rounded-lg bg-muted/30 p-3 space-y-2">
                  <div className="text-xs font-medium text-muted-foreground">
                    {t('optimizer.detectedVariablesLabel')}
                  </div>
                  {visibleVariables.length > 0 ? (
                    <div className="flex flex-wrap gap-2">
                      {visibleVariables.map(variable => (
                        <Badge key={variable} variant="outline">
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

          <div className="space-y-4">
            <div className="flex items-start justify-between gap-3">
              <div className="space-y-1">
                <Label>{t('optimizer.outputLabel')}</Label>
                <div className="text-xs text-muted-foreground">{t('optimizer.variantHint')}</div>
              </div>
              <div className="flex items-center gap-2">
                <Button variant="outline" onClick={() => setPlaygroundOpen(true)} disabled={!currentOutput}>
                  <FlaskConical className="h-4 w-4" />
                  {t('actions.testInPlayground')}
                </Button>
                <Button variant="outline" onClick={handleCopyCurrent} disabled={!currentOutput}>
                  <Copy className="h-4 w-4" />
                  {t('optimizer.copy')}
                </Button>
              </div>
            </div>

            {isStreaming ? (
              <div className="rounded-xl border p-6 space-y-4">
                <div className="text-sm font-medium">{t('optimizer.progress.title')}</div>
                <div className="space-y-3">
                  {optimizerProgressSteps.map((step, index) => {
                    const isDone = index < progressStepIndex;
                    const isCurrent = index === progressStepIndex;
                    return (
                      <div
                        key={step}
                        className={`rounded-lg border px-3 py-2 text-sm ${
                          isCurrent
                            ? 'border-primary bg-primary/5 text-foreground'
                            : isDone
                              ? 'border-border bg-muted/20 text-foreground'
                              : 'border-border/60 text-muted-foreground'
                        }`}
                      >
                        {t(`optimizer.progress.steps.${step}`)}
                      </div>
                    );
                  })}
                </div>
                <div className="text-sm text-muted-foreground">{t('optimizer.runningDescription')}</div>
                <Textarea value={streamOutput} readOnly className="min-h-[260px] font-mono text-xs" />
              </div>
            ) : !result ? (
              <div className="rounded-xl border border-dashed p-8 text-sm text-muted-foreground">
                {t('optimizer.emptyState')}
              </div>
            ) : (
              <div className="space-y-3">
                <div className="rounded-lg border bg-muted/20 p-3 text-sm text-muted-foreground">
                  {t(`optimizer.goals.${goal}.description`)}
                </div>
                <Textarea value={currentOutput} readOnly className="min-h-[430px] font-mono text-xs" />
              </div>
            )}
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel')}
          </Button>
          {onApplyResult ? (
            <Button onClick={handleApply} disabled={!currentOutput} loading={isApplying}>
              <WandSparkles className="h-4 w-4" />
              {applyLabel || t('optimizer.apply')}
            </Button>
          ) : null}
          <Button onClick={handleCopyCurrent} disabled={!currentOutput}>
            <Copy className="h-4 w-4" />
            {t('optimizer.copy')}
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
