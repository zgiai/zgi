'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import Link from 'next/link';
import {
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  CircleAlert,
  Copy,
  FileText,
  PencilLine,
  Play,
  WandSparkles,
} from 'lucide-react';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { ModelSelector, type ModelSelectorValue } from '@/components/common/model-selector';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { promptService } from '@/services/prompt.service';
import { useDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { usePrompt } from '@/hooks/prompt/use-prompts';
import type { PromptPlaygroundMessage } from '@/services/types/prompt';
import { extractPromptVariables } from './prompt-optimizer-template';
import { getPromptRuntimeErrorMessage } from './prompt-runtime-errors';
import { useWorkspaceStore } from '@/store/workspace-store';

const playgroundProgressSteps = ['analyze', 'rewrite', 'polish'] as const;
const zgiCapabilityTokenPattern =
  /^<zgi:(knowledge|skill|database|table|workflow)\b[\s\S]*<\/zgi:\1>$/;

interface PromptPlaygroundPanelProps {
  prefillPromptId?: string;
  prefillPromptText?: string;
  prefillPromptMessages?: PromptPlaygroundMessage[];
  prefillPromptLabel?: string;
  prefillModel?: ModelSelectorValue | null;
  onChoosePrompt?: () => void;
}

function normalizePromptMessages(
  content: Array<{ role: 'system' | 'user' | 'assistant'; content: string }>
): PromptPlaygroundMessage[] {
  return content.map(message => ({
    role: message.role,
    content: message.content,
  }));
}

function collectPromptVariables(prompt: string, messages: PromptPlaygroundMessage[]) {
  const toFillableVariables = (tokens: string[]) =>
    tokens.filter(token => !zgiCapabilityTokenPattern.test(token.trim()));

  if (messages.length > 0) {
    return Array.from(
      new Set(
        messages.flatMap(message => toFillableVariables(extractPromptVariables(message.content)))
      )
    );
  }
  return toFillableVariables(extractPromptVariables(prompt));
}

function normalizeVariableKey(token: string) {
  const trimmed = token.trim();
  if (trimmed.startsWith('${') && trimmed.endsWith('}')) {
    return trimmed.slice(2, -1).trim();
  }
  return trimmed
    .replace(/^\{\{#?/, '')
    .replace(/#?\}\}$/, '')
    .trim();
}

function isInputLikeVariable(token: string) {
  const normalized = normalizeVariableKey(token).toLowerCase();
  return (
    normalized === 'input' ||
    normalized === 'query' ||
    normalized === 'sys.query' ||
    normalized.endsWith('.input')
  );
}

function formatVariableTokenLabel(token: string, inputLabel: string) {
  const normalized = normalizeVariableKey(token);
  if (isInputLikeVariable(token)) {
    return inputLabel;
  }
  return normalized;
}

function renderPromptPreview(content: string, input: string, variables: Record<string, string>) {
  const replacements: Record<string, string> = { ...variables };
  if (input.trim()) {
    replacements.input = input;
    replacements.query = input;
    replacements['sys.query'] = input;
  }

  return content.replace(/\{\{#[^{}]+#\}\}|\{\{[^{}]+\}\}|\$\{[^{}]+\}/g, token => {
    const key = normalizeVariableKey(token);
    return Object.prototype.hasOwnProperty.call(replacements, key) ? replacements[key] : token;
  });
}

function hasPromptVariable(content: string) {
  return /\{\{#[^{}]+#\}\}|\{\{[^{}]+\}\}|\$\{[^{}]+\}/.test(content);
}

export function PromptPlaygroundPanel({
  prefillPromptId,
  prefillPromptText,
  prefillPromptMessages,
  prefillPromptLabel,
  prefillModel,
  onChoosePrompt,
}: PromptPlaygroundPanelProps) {
  const t = useT('prompts');
  const { value: defaultModel } = useDefaultModelByUseCase('text-chat');
  const { prompt: prefillPrompt } = usePrompt(prefillPromptId, !!prefillPromptId);
  const organizationRole = useWorkspaceStore.use.permissionState().organizationRole;
  const isAdminOrOwner = organizationRole === 'owner' || organizationRole === 'admin';
  const [prompt, setPrompt] = useState('');
  const [messageBlocks, setMessageBlocks] = useState<PromptPlaygroundMessage[]>([]);
  const [input, setInput] = useState('');
  const [selectedModel, setSelectedModel] = useState<ModelSelectorValue | null>(null);
  const [isRunning, setIsRunning] = useState(false);
  const [progressStepIndex, setProgressStepIndex] = useState(0);
  const [output, setOutput] = useState('');
  const [runError, setRunError] = useState('');
  const [runErrorHint, setRunErrorHint] = useState('');
  const [runErrorDetails, setRunErrorDetails] = useState('');
  const [detectedVariables, setDetectedVariables] = useState<string[]>([]);
  const [renderedPrompt, setRenderedPrompt] = useState('');
  const [renderedMessages, setRenderedMessages] = useState<PromptPlaygroundMessage[]>([]);
  const [renderedPromptFallback, setRenderedPromptFallback] = useState('');
  const [renderedMessagesFallback, setRenderedMessagesFallback] = useState<
    PromptPlaygroundMessage[]
  >([]);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [detailsOpen, setDetailsOpen] = useState(false);
  const appliedPrefillKeyRef = useRef<string | null>(null);
  const appliedModelKeyRef = useRef<string | null>(null);
  const modelSelectorContainerRef = useRef<HTMLDivElement>(null);
  const outputCardRef = useRef<HTMLDivElement>(null);
  const hasAutoScrolledToOutputRef = useRef(false);
  const streamHandleRef = useRef<{ close: () => void } | null>(null);

  useEffect(() => {
    if (prefillModel?.provider && prefillModel?.model) {
      const nextKey = `${prefillModel.provider}:${prefillModel.model}`;
      if (appliedModelKeyRef.current !== nextKey) {
        appliedModelKeyRef.current = nextKey;
        setSelectedModel(prefillModel);
      }
      return;
    }
    if (selectedModel) return;
    if (defaultModel?.provider && defaultModel?.model) {
      appliedModelKeyRef.current = `${defaultModel.provider}:${defaultModel.model}`;
      setSelectedModel({
        provider: defaultModel.provider,
        model: defaultModel.model,
      });
    }
  }, [defaultModel?.model, defaultModel?.provider, prefillModel, selectedModel]);

  useEffect(() => {
    return () => {
      streamHandleRef.current?.close();
      streamHandleRef.current = null;
    };
  }, []);

  useEffect(() => {
    const hasPrefill =
      !!prefillPromptId || !!prefillPromptText?.trim() || (prefillPromptMessages?.length ?? 0) > 0;
    setAdvancedOpen(!hasPrefill);
  }, [prefillPromptId, prefillPromptMessages, prefillPromptText]);

  useEffect(() => {
    if (!prefillPromptId && !prefillPromptText?.trim() && !(prefillPromptMessages?.length ?? 0)) {
      appliedPrefillKeyRef.current = null;
      return;
    }
  }, [prefillPromptId, prefillPromptMessages, prefillPromptText]);

  useEffect(() => {
    if (!prefillPromptMessages?.length) {
      return;
    }

    const nextKey = `messages:${JSON.stringify(prefillPromptMessages)}`;
    if (appliedPrefillKeyRef.current === nextKey) {
      return;
    }

    setMessageBlocks(prefillPromptMessages);
    setPrompt('');
    setInput('');
    setCustomVariables({});
    setOutput('');
    setRunError('');
    setRunErrorHint('');
    setRunErrorDetails('');
    setRenderedPrompt('');
    setRenderedMessages([]);
    setRenderedPromptFallback('');
    setRenderedMessagesFallback([]);
    setDetectedVariables([]);
    setProgressStepIndex(0);
    appliedPrefillKeyRef.current = nextKey;
  }, [prefillPromptMessages]);

  useEffect(() => {
    const normalizedPrefillText = prefillPromptText?.trim();
    if (!normalizedPrefillText || (prefillPromptMessages?.length ?? 0) > 0) {
      return;
    }

    const nextKey = `text:${normalizedPrefillText}`;
    if (appliedPrefillKeyRef.current === nextKey) {
      return;
    }

    setPrompt(prefillPromptText ?? '');
    setMessageBlocks([]);
    setInput('');
    setCustomVariables({});
    setOutput('');
    setRenderedPrompt('');
    setRenderedMessages([]);
    setRenderedPromptFallback('');
    setRenderedMessagesFallback([]);
    setDetectedVariables([]);
    setProgressStepIndex(0);
    appliedPrefillKeyRef.current = nextKey;
  }, [prefillPromptMessages, prefillPromptText]);

  useEffect(() => {
    if (prefillPromptText?.trim() || (prefillPromptMessages?.length ?? 0) > 0) {
      return;
    }
    if (!prefillPromptId || !prefillPrompt) {
      return;
    }

    const latestVersion = prefillPrompt.versions[0];
    if (!latestVersion) {
      return;
    }

    const nextKey = `prompt:${prefillPrompt.id}:${latestVersion.id}`;
    if (appliedPrefillKeyRef.current === nextKey) {
      return;
    }

    if (typeof latestVersion.content === 'string') {
      setPrompt(latestVersion.content);
      setMessageBlocks([]);
    } else {
      setMessageBlocks(normalizePromptMessages(latestVersion.content));
      setPrompt('');
    }
    setInput('');
    setCustomVariables({});
    setOutput('');
    setRenderedPrompt('');
    setRenderedMessages([]);
    setRenderedPromptFallback('');
    setRenderedMessagesFallback([]);
    setDetectedVariables([]);
    setProgressStepIndex(0);
    appliedPrefillKeyRef.current = nextKey;
  }, [prefillPrompt, prefillPromptId, prefillPromptMessages, prefillPromptText]);

  const allVariables = useMemo(
    () => collectPromptVariables(prompt, messageBlocks),
    [messageBlocks, prompt]
  );

  const variables = useMemo(() => {
    return allVariables.filter(variable => !isInputLikeVariable(variable));
  }, [allVariables]);

  const variableMap = useMemo(() => {
    return variables.reduce<Record<string, string>>((acc, token) => {
      const normalized = normalizeVariableKey(token);
      acc[normalized] = '';
      return acc;
    }, {});
  }, [variables]);

  const [customVariables, setCustomVariables] = useState<Record<string, string>>({});

  useEffect(() => {
    setCustomVariables(previous => {
      const next: Record<string, string> = {};
      for (const key of Object.keys(variableMap)) {
        next[key] = previous[key] ?? '';
      }
      return next;
    });
  }, [variableMap]);

  const missingVariables = useMemo(() => {
    return variables.filter(token => {
      const key = normalizeVariableKey(token);
      return !(customVariables[key] ?? '').trim();
    });
  }, [customVariables, variables]);
  const simpleInputTokens = useMemo(
    () => allVariables.filter(token => isInputLikeVariable(token)),
    [allVariables]
  );
  const advancedVariables = useMemo(
    () => variables.filter(token => !simpleInputTokens.includes(token)),
    [simpleInputTokens, variables]
  );
  const missingAdvancedVariables = useMemo(
    () => missingVariables.filter(token => !isInputLikeVariable(token)),
    [missingVariables]
  );
  const requiresTestInput = useMemo(
    () => allVariables.some(variable => isInputLikeVariable(variable)),
    [allVariables]
  );

  const handleRun = async () => {
    if (!prompt.trim() && messageBlocks.length === 0) return;
    setIsRunning(true);
    setProgressStepIndex(0);
    hasAutoScrolledToOutputRef.current = false;
    setOutput('');
    setRunError('');
    setRunErrorHint('');
    setRunErrorDetails('');
    setRenderedPrompt('');
    setRenderedMessages([]);
    setRenderedPromptFallback('');
    setRenderedMessagesFallback([]);
    setDetectedVariables([]);
    streamHandleRef.current?.close();
    streamHandleRef.current = null;

    const scrollOutputIntoView = () => {
      if (hasAutoScrolledToOutputRef.current) return;
      hasAutoScrolledToOutputRef.current = true;
      window.requestAnimationFrame(() => {
        outputCardRef.current?.scrollIntoView({
          block: 'start',
          behavior: 'smooth',
        });
      });
    };

    try {
      const resolvedVariables = {
        ...customVariables,
        ...Object.fromEntries(simpleInputTokens.map(token => [normalizeVariableKey(token), input])),
      };
      setRenderedPromptFallback(renderPromptPreview(prompt, input, resolvedVariables));
      setRenderedMessagesFallback(
        messageBlocks.map(message => ({
          ...message,
          content: renderPromptPreview(message.content, input, resolvedVariables),
        }))
      );
      streamHandleRef.current = await promptService.streamPlaygroundPrompt(
        {
          prompt,
          messages: messageBlocks.length > 0 ? messageBlocks : undefined,
          input,
          provider: selectedModel?.provider,
          model: selectedModel?.model,
          variables: resolvedVariables,
        },
        {
          onProgress: payload => {
            const step = payload.step;
            const nextIndex = playgroundProgressSteps.findIndex(item => item === step);
            if (nextIndex >= 0) {
              setProgressStepIndex(nextIndex);
            }
          },
          onMeta: payload => {
            setDetectedVariables(payload.detected_variables ?? []);
            setRenderedPrompt(payload.rendered_prompt ?? '');
            setRenderedMessages(payload.rendered_messages ?? []);
          },
          onChunk: payload => {
            if (typeof payload.text === 'string') {
              setOutput(previous => `${previous}${payload.text}`);
              if (payload.text.trim()) {
                scrollOutputIntoView();
              }
            }
          },
          onDone: payload => {
            setOutput(payload.output ?? '');
            setRenderedPrompt(payload.rendered_prompt ?? '');
            setRenderedMessages(payload.rendered_messages ?? []);
            setDetectedVariables(payload.detected_variables ?? []);
            setProgressStepIndex(playgroundProgressSteps.length - 1);
            setIsRunning(false);
            streamHandleRef.current = null;
            if ((payload.output ?? '').trim()) {
              scrollOutputIntoView();
            }
          },
          onError: error => {
            const modelLabel = selectedModel?.model
              ? selectedModel.provider
                ? `${selectedModel.provider} / ${selectedModel.model}`
                : selectedModel.model
              : undefined;
            const normalized = getPromptRuntimeErrorMessage(
              error,
              modelLabel,
              isAdminOrOwner,
              (key, values) => t(key as never, values as never)
            );
            setRunError(normalized.message);
            setRunErrorHint(normalized.hint);
            setRunErrorDetails(normalized.details);
            toast.error(normalized.message);
            setIsRunning(false);
            streamHandleRef.current = null;
          },
          onClose: () => {
            setIsRunning(false);
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
      const normalized = getPromptRuntimeErrorMessage(
        error,
        modelLabel,
        isAdminOrOwner,
        (key, values) => t(key as never, values as never)
      );
      setRunError(normalized.message);
      setRunErrorHint(normalized.hint);
      setRunErrorDetails(normalized.details);
      toast.error(normalized.message);
      setIsRunning(false);
    }
  };

  const handleFocusModelSelector = () => {
    modelSelectorContainerRef.current?.scrollIntoView({
      block: 'center',
      behavior: 'smooth',
    });
    const trigger = modelSelectorContainerRef.current?.querySelector<HTMLElement>(
      'button,[role="combobox"]'
    );
    trigger?.focus();
  };

  const handleCopyOutput = async () => {
    const normalizedOutput = output.trim();
    if (!normalizedOutput) return;
    try {
      await navigator.clipboard.writeText(normalizedOutput);
      toast.success(t('messages.playgroundOutputCopied'));
    } catch {
      toast.error(t('messages.playgroundOutputCopyFailed'));
    }
  };

  const prefilledFromLabel = prefillPromptLabel || prefillPrompt?.name;
  const showRuntimePanels = isRunning || !!output.trim();
  const showImprovementActions = Boolean(prefillPromptId && output.trim());
  const canEditPrefilledPrompt =
    showImprovementActions && Boolean(prefillPrompt && prefillPrompt.source !== 'official');
  const displayedRenderedPrompt =
    renderedPrompt.trim() && !hasPromptVariable(renderedPrompt)
      ? renderedPrompt
      : renderedPromptFallback;
  const displayedRenderedMessages =
    renderedMessages.length > 0
      ? renderedMessages.some(message => hasPromptVariable(message.content))
        ? renderedMessagesFallback
        : renderedMessages
      : renderedMessagesFallback;
  const showRenderedDetails =
    renderedMessages.length > 0 ||
    !!renderedPrompt ||
    (!!output.trim() && (displayedRenderedMessages.length > 0 || !!displayedRenderedPrompt.trim()));
  const hasPromptContent = messageBlocks.length > 0 || !!prompt.trim();
  const inputVariableLabel = t('playground.inputLabel');
  const cannotRunReason = !hasPromptContent
    ? t('playground.missingPromptHint')
    : requiresTestInput && !input.trim()
      ? t('playground.missingInputHint')
      : missingAdvancedVariables.length > 0
        ? `${t('playground.missingVariablesHint')} ${missingAdvancedVariables
            .map(token => formatVariableTokenLabel(token, inputVariableLabel))
            .join('，')}`
        : '';
  const canRun = hasPromptContent && !cannotRunReason;
  const readinessDescription = canRun ? t('playground.readinessReadyDescription') : cannotRunReason;

  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1fr)_420px]">
      <section className="rounded-2xl border bg-background p-4 shadow-sm">
        <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0">
            <h3 className="text-lg font-semibold">{t('playground.inputsTitle')}</h3>
            {prefilledFromLabel ? (
              <div className="mt-1 truncate text-sm text-muted-foreground">
                {t('playground.prefilledFrom')}{' '}
                <span className="font-medium text-foreground">{prefilledFromLabel}</span>
              </div>
            ) : null}
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {onChoosePrompt ? (
              <Button size="sm" variant="outline" onClick={onChoosePrompt}>
                <FileText className="h-4 w-4" />
                {prefilledFromLabel ? t('playground.changePrompt') : t('playground.choosePrompt')}
              </Button>
            ) : null}
            <Badge variant={canRun ? 'default' : 'outline'}>
              {canRun ? t('playground.runStatusReady') : t('playground.runStatusBlocked')}
            </Badge>
          </div>
        </div>

        <div className="space-y-4">
          <div ref={modelSelectorContainerRef} className="space-y-2">
            <Label>{t('playground.modelLabel')}</Label>
            <ModelSelector
              modelType="text-chat"
              value={selectedModel ?? undefined}
              onChange={value => setSelectedModel(value)}
            />
          </div>

          <div className="space-y-2">
            <Label>{t('playground.inputLabel')}</Label>
            <Textarea
              value={input}
              onChange={e => setInput(e.target.value)}
              placeholder={t('playground.inputPlaceholder')}
              className="min-h-[112px]"
            />
            <div className="flex items-start gap-2 text-xs text-muted-foreground">
              {canRun ? (
                <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 text-primary" />
              ) : (
                <CircleAlert className="mt-0.5 h-3.5 w-3.5" />
              )}
              <span>{readinessDescription}</span>
            </div>
          </div>

          <Button onClick={handleRun} disabled={isRunning || !canRun} className="w-full">
            <Play className="h-4 w-4" />
            {isRunning ? t('playground.running') : t('playground.run')}
          </Button>

          <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
            <div className="rounded-xl border">
              <CollapsibleTrigger asChild>
                <Button variant="ghost" className="w-full justify-between rounded-xl px-4 py-3">
                  <span>{t('playground.advancedTitle')}</span>
                  {advancedOpen ? (
                    <ChevronDown className="h-4 w-4" />
                  ) : (
                    <ChevronRight className="h-4 w-4" />
                  )}
                </Button>
              </CollapsibleTrigger>
              <CollapsibleContent className="border-t px-4 py-4 space-y-4">
                <div className="text-sm text-muted-foreground">
                  {t('playground.advancedDescription')}
                </div>
                <div className="space-y-2">
                  <Label>
                    {messageBlocks.length > 0
                      ? t('playground.messagesLabel')
                      : t('playground.promptLabel')}
                  </Label>
                  {messageBlocks.length > 0 ? (
                    <div className="space-y-3 rounded-xl border p-4">
                      {messageBlocks.map((message, index) => (
                        <div key={`${message.role}-${index}`} className="space-y-2">
                          <Badge variant="outline">
                            {t(`playground.messageRoles.${message.role}`)}
                          </Badge>
                          <Textarea
                            value={message.content}
                            onChange={e =>
                              setMessageBlocks(previous =>
                                previous.map((item, itemIndex) =>
                                  itemIndex === index
                                    ? {
                                        ...item,
                                        content: e.target.value,
                                      }
                                    : item
                                )
                              )
                            }
                            className="min-h-[120px] font-mono text-xs"
                          />
                        </div>
                      ))}
                    </div>
                  ) : (
                    <Textarea
                      value={prompt}
                      onChange={e => setPrompt(e.target.value)}
                      placeholder={t('playground.promptPlaceholder')}
                      className="min-h-[220px] font-mono text-xs"
                    />
                  )}
                </div>
                {advancedVariables.length > 0 ? (
                  <div className="space-y-3 rounded-xl border p-4">
                    <div className="text-sm font-medium">{t('playground.variablesTitle')}</div>
                    <div className="space-y-3">
                      {advancedVariables.map(token => {
                        const key = normalizeVariableKey(token);
                        return (
                          <div key={token} className="space-y-2">
                            <Label>{formatVariableTokenLabel(token, inputVariableLabel)}</Label>
                            <Textarea
                              value={customVariables[key] ?? ''}
                              onChange={e =>
                                setCustomVariables(previous => ({
                                  ...previous,
                                  [key]: e.target.value,
                                }))
                              }
                              className="min-h-[80px]"
                            />
                          </div>
                        );
                      })}
                    </div>
                  </div>
                ) : null}
              </CollapsibleContent>
            </div>
          </Collapsible>
        </div>
      </section>

      <section ref={outputCardRef} className="rounded-2xl border bg-background p-4 shadow-sm">
        <div className="mb-4 flex items-start justify-between gap-3">
          <div>
            <h3 className="text-lg font-semibold">{t('playground.outputTitle')}</h3>
            <div className="mt-1 text-sm text-muted-foreground">
              {output.trim()
                ? t('playground.feedbackReadyDescription')
                : t('playground.feedbackEmptyDescription')}
            </div>
          </div>
          {output.trim() ? (
            <div className="flex flex-wrap items-center justify-end gap-2">
              {showImprovementActions ? (
                <>
                  {canEditPrefilledPrompt ? (
                    <Button asChild size="sm" variant="outline">
                      <Link href={`/console/prompts/${prefillPromptId}?action=edit`}>
                        <PencilLine className="h-4 w-4" />
                        {t('playground.editPromptAction')}
                      </Link>
                    </Button>
                  ) : null}
                  <Button asChild size="sm" variant="outline">
                    <Link href={`/console/prompts/${prefillPromptId}?action=optimize`}>
                      <WandSparkles className="h-4 w-4" />
                      {t('playground.optimizePromptAction')}
                    </Link>
                  </Button>
                </>
              ) : null}
              <Button size="sm" variant="outline" onClick={() => void handleCopyOutput()}>
                <Copy className="h-4 w-4" />
                {t('playground.copyOutput')}
              </Button>
            </div>
          ) : null}
        </div>

        <div className="space-y-4">
          {showRuntimePanels ? (
            <div className="grid grid-cols-3 gap-2">
              {playgroundProgressSteps.map((step, index) => {
                const isDone = index < progressStepIndex;
                const isCurrent = isRunning && index === progressStepIndex;
                return (
                  <div
                    key={step}
                    className={`rounded-lg border px-2 py-2 text-center text-xs ${
                      isCurrent
                        ? 'border-primary bg-primary/5 text-foreground'
                        : isDone
                          ? 'border-border bg-muted/20 text-foreground'
                          : 'border-border/60 text-muted-foreground'
                    }`}
                  >
                    {t(`playground.progress.steps.${step}`)}
                  </div>
                );
              })}
            </div>
          ) : null}

          <div className="space-y-2">
            <Label>{t('playground.resultLabel')}</Label>
            <Textarea
              value={output}
              readOnly
              placeholder={t('playground.resultPlaceholder')}
              className="min-h-[320px] text-sm leading-6"
            />
          </div>

          {runError ? (
            <div className="rounded-xl border border-destructive/20 bg-destructive/5 px-4 py-3 space-y-2">
              <div className="text-sm font-medium text-destructive">
                {t('playground.runErrorTitle')}
              </div>
              <div className="text-sm text-muted-foreground">{runError}</div>
              {runErrorHint ? (
                <div className="text-sm text-muted-foreground">{runErrorHint}</div>
              ) : null}
              {runErrorHint ? (
                <div className="flex items-center gap-2 flex-wrap">
                  <Button size="sm" variant="outline" onClick={handleFocusModelSelector}>
                    {t('messages.providerActionSwitchModel')}
                  </Button>
                  {isAdminOrOwner ? (
                    <Button asChild size="sm" variant="outline">
                      <Link href="/dashboard/provider">
                        {t('messages.providerActionManageChannels')}
                      </Link>
                    </Button>
                  ) : null}
                </div>
              ) : null}
              {runErrorDetails && runErrorDetails !== runError ? (
                <Collapsible>
                  <CollapsibleTrigger asChild>
                    <Button variant="ghost" size="sm" className="px-0 text-xs">
                      {t('playground.showErrorDetails')}
                    </Button>
                  </CollapsibleTrigger>
                  <CollapsibleContent className="pt-2">
                    <Textarea
                      value={runErrorDetails}
                      readOnly
                      className="min-h-[140px] font-mono text-xs"
                    />
                  </CollapsibleContent>
                </Collapsible>
              ) : null}
            </div>
          ) : null}

          {showRenderedDetails ? (
            <Collapsible open={detailsOpen} onOpenChange={setDetailsOpen}>
              <div className="rounded-xl border">
                <CollapsibleTrigger asChild>
                  <Button variant="ghost" className="w-full justify-between rounded-xl px-4 py-3">
                    <span>{t('playground.detailsTitle')}</span>
                    {detailsOpen ? (
                      <ChevronDown className="h-4 w-4" />
                    ) : (
                      <ChevronRight className="h-4 w-4" />
                    )}
                  </Button>
                </CollapsibleTrigger>
                <CollapsibleContent className="border-t px-4 py-4 space-y-4">
                  {detectedVariables.length > 0 ? (
                    <div className="flex flex-wrap gap-2">
                      {detectedVariables.map(variable => (
                        <Badge key={variable} variant="outline">
                          {variable}
                        </Badge>
                      ))}
                    </div>
                  ) : null}

                  {displayedRenderedMessages.length > 0 ? (
                    <div className="space-y-2">
                      <Label>{t('playground.renderedMessagesLabel')}</Label>
                      <div className="space-y-3 rounded-xl border p-4">
                        {displayedRenderedMessages.map((message, index) => (
                          <div key={`${message.role}-rendered-${index}`} className="space-y-2">
                            <Badge variant="outline">
                              {t(`playground.messageRoles.${message.role}`)}
                            </Badge>
                            <Textarea
                              value={message.content}
                              readOnly
                              className="min-h-[100px] font-mono text-xs"
                            />
                          </div>
                        ))}
                      </div>
                    </div>
                  ) : displayedRenderedPrompt.trim() ? (
                    <div className="space-y-2">
                      <Label>{t('playground.renderedPromptLabel')}</Label>
                      <Textarea
                        value={displayedRenderedPrompt}
                        readOnly
                        className="min-h-[140px] font-mono text-xs"
                      />
                    </div>
                  ) : null}
                </CollapsibleContent>
              </div>
            </Collapsible>
          ) : null}
        </div>
      </section>
    </div>
  );
}

export default PromptPlaygroundPanel;
