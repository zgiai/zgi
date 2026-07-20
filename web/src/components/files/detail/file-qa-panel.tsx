'use client';

import { useEffect, useRef, useState } from 'react';
import type { FormEvent, KeyboardEvent } from 'react';
import {
  AlertCircle,
  Bot,
  ChevronDown,
  ChevronRight,
  Layers3,
  LocateFixed,
  Loader2,
  MessageSquareText,
  Send,
  Trash2,
  User,
} from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { Textarea } from '@/components/ui/textarea';
import { ModelSelector, type ModelSelectorValue } from '@/components/common/model-selector';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { useDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { useT } from '@/i18n';
import { fileManageService } from '@/services/file-manage.service';
import type {
  AskFileQuestionResponse,
  FileAssetArtifactState,
  FileDetailProcessing,
  FileQuestionHistoryMessage,
  FileQuestionAnswerSource,
  FileQuestionStreamEvent,
} from '@/services/types/file';

interface FileQAPanelProps {
  fileId: string;
  artifactState?: FileAssetArtifactState;
  processing?: FileDetailProcessing;
  vectorStatus?: string;
  enabled: boolean;
  preparingIndex?: boolean;
  prepareError?: string | null;
  onLocateSource?: (source: FileQuestionAnswerSource) => void;
}

interface FileQAExchange {
  id: string;
  question: string;
  result: AskFileQuestionResponse;
  streaming?: boolean;
}

const MARKDOWN_IMAGE_PATTERN = /!\[([^\]\r\n]*)\]\(([^)\r\n]+)\)/g;

function isComposingEnterEvent(event: KeyboardEvent<HTMLTextAreaElement>): boolean {
  const nativeEvent = event.nativeEvent as globalThis.KeyboardEvent & {
    isComposing?: boolean;
  };

  return nativeEvent.isComposing === true || event.keyCode === 229;
}

export function FileQAPanel({
  fileId,
  enabled,
  preparingIndex = false,
  prepareError = null,
  onLocateSource,
}: FileQAPanelProps) {
  const t = useT('files');
  const [question, setQuestion] = useState('');
  const [exchanges, setExchanges] = useState<FileQAExchange[]>([]);
  const [isStreaming, setIsStreaming] = useState(false);
  const [qaError, setQaError] = useState<string | null>(null);
  const [answerModelSelection, setAnswerModelSelection] = useState<ModelSelectorValue | null>(null);
  const isComposingRef = useRef(false);
  const abortRef = useRef<AbortController | null>(null);
  const closeRef = useRef<(() => void) | null>(null);
  const { value: defaultAnswerModel } = useDefaultModelByUseCase('text-chat');
  const selectedAnswerModel = answerModelSelection ?? defaultAnswerModel ?? null;
  const canSubmit = enabled && !preparingIndex && !prepareError && question.trim().length > 0 && !isStreaming;

  useEffect(() => {
    return () => {
      closeRef.current?.();
      abortRef.current?.abort();
    };
  }, []);

  const updateExchange = (id: string, updater: (exchange: FileQAExchange) => FileQAExchange) => {
    setExchanges(prev => prev.map(exchange => (exchange.id === id ? updater(exchange) : exchange)));
  };

  const buildQuestionHistory = (): FileQuestionHistoryMessage[] =>
    exchanges
      .filter(exchange => !exchange.streaming && exchange.result.answer.trim().length > 0)
      .slice(-6)
      .map(exchange => ({
        question: exchange.question,
        answer: exchange.result.answer,
      }));

  const clearConversation = () => {
    setExchanges([]);
    setQaError(null);
  };

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const trimmed = question.trim();
    if (!trimmed || !enabled || preparingIndex || prepareError) return;
    const exchangeId = `${Date.now()}-${exchanges.length}`;
    const controller = new AbortController();
    abortRef.current?.abort();
    closeRef.current?.();
    abortRef.current = controller;
    closeRef.current = null;
    setQaError(null);
    setIsStreaming(true);
    setExchanges(prev => [
      ...prev,
      {
        id: exchangeId,
        question: trimmed,
        streaming: true,
        result: {
          answer: '',
          sources: [],
          retrieval: {
            top_k: 6,
            hit_count: 0,
            primary_hit_count: 0,
          },
        },
      },
    ]);

    const handleStreamEvent = (payload: FileQuestionStreamEvent) => {
      if (payload.type === 'retrieval') {
        updateExchange(exchangeId, exchange => ({
          ...exchange,
          result: {
            ...exchange.result,
            sources: payload.sources ?? exchange.result.sources,
            retrieval: payload.retrieval ?? exchange.result.retrieval,
          },
        }));
        return;
      }
      if (payload.type === 'delta') {
        updateExchange(exchangeId, exchange => ({
          ...exchange,
          result: {
            ...exchange.result,
            answer: `${exchange.result.answer}${payload.delta ?? ''}`,
          },
        }));
        return;
      }
      if (payload.type === 'done') {
        updateExchange(exchangeId, exchange => ({
          ...exchange,
          streaming: false,
          result: {
            answer: payload.answer ?? exchange.result.answer,
            sources: payload.sources ?? exchange.result.sources,
            retrieval: payload.retrieval ?? exchange.result.retrieval,
          },
        }));
        setQuestion('');
        setIsStreaming(false);
        closeRef.current = null;
        abortRef.current = null;
        return;
      }
      if (payload.type === 'error') {
        const message = payload.error || t('detail.qa.askFailed');
        setQaError(message);
        updateExchange(exchangeId, exchange => ({ ...exchange, streaming: false }));
        setIsStreaming(false);
        closeRef.current = null;
        abortRef.current = null;
      }
    };

    try {
      const stream = await fileManageService.streamFileQuestion(
        fileId,
        {
          question: trimmed,
          top_k: 6,
          history: buildQuestionHistory(),
          ...(selectedAnswerModel
            ? {
                answer_model_provider: selectedAnswerModel.provider,
                answer_model: selectedAnswerModel.model,
              }
            : {}),
        },
        {
          abortSignal: controller.signal,
          onEvent: handleStreamEvent,
          onError: error => {
            const message = error.message || t('detail.qa.askFailed');
            setQaError(message);
            updateExchange(exchangeId, exchange => ({ ...exchange, streaming: false }));
            setIsStreaming(false);
          },
          onClose: () => {
            closeRef.current = null;
            abortRef.current = null;
          },
        }
      );
      closeRef.current = stream.close;
    } catch (error) {
      const message = error instanceof Error ? error.message : t('detail.qa.askFailed');
      setQaError(message);
      updateExchange(exchangeId, exchange => ({ ...exchange, streaming: false }));
      setIsStreaming(false);
    }
  };

  const handleQuestionKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key !== 'Enter' || event.shiftKey) return;
    if (isComposingRef.current || isComposingEnterEvent(event)) return;

    event.preventDefault();
    if (!canSubmit) return;
    event.currentTarget.form?.requestSubmit();
  };

  if (!enabled) {
    return (
      <Alert className="rounded-md border-border bg-background px-4 py-3 [&>svg]:left-4 [&>svg]:top-3.5 [&>svg~*]:pl-7">
        <AlertCircle className="h-4 w-4" />
        <AlertTitle className="mb-1 text-sm leading-5">{t('detail.qa.notReadyTitle')}</AlertTitle>
        <AlertDescription className="text-sm leading-5 text-muted-foreground">
          {t('detail.qa.notReadyDescription')}
        </AlertDescription>
      </Alert>
    );
  }

  if (preparingIndex) {
    return (
      <Alert className="rounded-md border-border bg-background px-4 py-3 [&>svg]:left-4 [&>svg]:top-3.5 [&>svg~*]:pl-7">
        <Loader2 className="h-4 w-4 animate-spin" />
        <AlertTitle className="mb-1 text-sm leading-5">
          {t('detail.qa.preparingTitle')}
        </AlertTitle>
        <AlertDescription className="text-sm leading-5 text-muted-foreground">
          {t('detail.qa.preparingDescription')}
        </AlertDescription>
      </Alert>
    );
  }

  if (prepareError) {
    return (
      <Alert
        variant="destructive"
        className="rounded-md px-4 py-3 [&>svg]:left-4 [&>svg]:top-3.5 [&>svg~*]:pl-7"
      >
        <AlertCircle className="h-4 w-4" />
        <AlertTitle className="mb-1 text-sm leading-5">
          {t('detail.qa.prepareFailedTitle')}
        </AlertTitle>
        <AlertDescription className="text-sm leading-5">{prepareError}</AlertDescription>
      </Alert>
    );
  }

  return (
    <div className="flex min-h-[520px] flex-col rounded-md border border-border bg-background">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border px-5 py-4">
        <div>
          <h2 className="text-base font-semibold text-foreground">{t('detail.qa.title')}</h2>
          <p className="mt-1 text-sm text-muted-foreground">{t('detail.qa.description')}</p>
        </div>
        <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-9 gap-1.5"
            onClick={clearConversation}
            disabled={isStreaming || exchanges.length === 0}
          >
            <Trash2 className="h-4 w-4" />
            {t('detail.qa.clearConversation')}
          </Button>
          <span className="text-sm text-muted-foreground">{t('detail.qa.answerModel')}</span>
          <ModelSelector
            modelType="text-chat"
            value={selectedAnswerModel ?? undefined}
            onChange={setAnswerModelSelection}
            placeholder={t('detail.qa.defaultAnswerModel')}
            disabled={isStreaming}
            className="h-9 w-[220px] bg-background"
          />
        </div>
      </div>

      <div className="min-h-0 flex-1 space-y-4 overflow-y-auto bg-muted/20 p-5">
        {exchanges.length === 0 ? (
          <div className="flex min-h-[260px] flex-col items-center justify-center text-center">
            <div className="flex h-12 w-12 items-center justify-center rounded-full bg-primary/10 text-primary">
              <MessageSquareText className="h-6 w-6" />
            </div>
            <h3 className="mt-4 text-base font-semibold text-foreground">
              {t('detail.qa.emptyTitle')}
            </h3>
          </div>
        ) : (
          exchanges.map(exchange => (
            <div key={exchange.id} className="space-y-3">
              <div className="flex justify-end">
                <div className="max-w-3xl rounded-md bg-primary px-4 py-3 text-sm leading-6 text-primary-foreground">
                  <div className="mb-1 flex items-center gap-2 text-xs opacity-80">
                    <User className="h-3.5 w-3.5" />
                    {t('detail.qa.question')}
                  </div>
                  {exchange.question}
                </div>
              </div>
              <div className="max-w-4xl rounded-md border border-border bg-background p-4">
                <div className="mb-3 flex items-center gap-2 text-sm font-medium text-foreground">
                  <Bot className="h-4 w-4 text-primary" />
                  {t('detail.qa.answer')}
                </div>
                <div className="text-sm leading-7 text-foreground">
                  {exchange.result.answer || exchange.streaming ? (
                    <>
                      <MarkdownViewer
                        content={buildFileQAAnswerMarkdown(exchange.result)}
                        isStreaming={exchange.streaming}
                        renderIdentity={`file-qa-${exchange.id}`}
                        className="text-sm leading-7 [&_img]:max-h-[360px] [&_img]:rounded-md"
                      />
                      {exchange.streaming ? (
                        <span className="ml-2 inline-flex items-center gap-1 text-muted-foreground">
                          <Loader2 className="h-3.5 w-3.5 animate-spin" />
                          {t('detail.qa.generating')}
                        </span>
                      ) : null}
                    </>
                  ) : (
                    t('detail.qa.askFailed')
                  )}
                </div>
                {!exchange.streaming ? (
                  <SourceList result={exchange.result} onLocateSource={onLocateSource} />
                ) : null}
              </div>
            </div>
          ))
        )}
        {qaError ? (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>{t('detail.qa.askFailedTitle')}</AlertTitle>
            <AlertDescription>{qaError}</AlertDescription>
          </Alert>
        ) : null}
      </div>

      <form onSubmit={handleSubmit} className="border-t border-border bg-background p-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <Textarea
            className="min-h-[84px] flex-1 resize-none border-muted-foreground/20 bg-muted/10 shadow-[0_1px_3px_rgba(15,23,42,0.08)] placeholder:font-medium placeholder:text-muted-foreground/75 hover:border-primary/30 focus-visible:border-primary/50 focus-visible:ring-2 focus-visible:ring-primary/10"
            value={question}
            onChange={event => setQuestion(event.target.value)}
            onCompositionStart={() => {
              isComposingRef.current = true;
            }}
            onCompositionEnd={() => {
              isComposingRef.current = false;
            }}
            onKeyDown={handleQuestionKeyDown}
            placeholder={t('detail.qa.placeholder')}
            disabled={isStreaming}
          />
          <Button
            type="submit"
            className="gap-2 border-primary/25 shadow-[0_1px_3px_rgba(15,23,42,0.08)] disabled:shadow-none sm:h-[84px]"
            disabled={!canSubmit}
          >
            {isStreaming ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Send className="h-4 w-4" />
            )}
            {t('detail.qa.send')}
          </Button>
        </div>
      </form>
    </div>
  );
}

function buildFileQAAnswerMarkdown(result: AskFileQuestionResponse): string {
  const answer = result.answer || '';
  const sourceImageMarkdown = extractSourceImageMarkdown(result.sources);
  if (!sourceImageMarkdown) {
    return answer;
  }
  return [answer, sourceImageMarkdown].filter(Boolean).join('\n\n');
}

function extractSourceImageMarkdown(sources: FileQuestionAnswerSource[]): string {
  const seen = new Set<string>();
  const images: string[] = [];
  const collect = (content?: string) => {
    if (!content) return;
    for (const match of content.matchAll(MARKDOWN_IMAGE_PATTERN)) {
      const alt = match[1]?.trim() || 'image';
      const src = match[2]?.trim();
      if (!src || seen.has(src)) continue;
      seen.add(src);
      images.push(`![${alt}](${src})`);
    }
  };

  sources.forEach(source => {
    collect(source.content);
    collect(source.snippet);
    source.children.forEach(child => {
      collect(child.content);
      collect(child.snippet);
    });
  });

  return images.join('\n\n');
}

function SourceList({
  result,
  onLocateSource,
}: {
  result: AskFileQuestionResponse;
  onLocateSource?: (source: FileQuestionAnswerSource) => void;
}) {
  const t = useT('files');
  const [open, setOpen] = useState(false);
  if (!result.sources.length) {
    return (
      <div className="mt-4 rounded-md border border-dashed border-border bg-muted/30 px-3 py-2 text-sm text-muted-foreground">
        {t('detail.qa.noSources')}
      </div>
    );
  }
  return (
    <Collapsible open={open} onOpenChange={setOpen} className="mt-4 space-y-2">
      <CollapsibleTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-8 gap-2 px-0 text-sm font-medium text-foreground hover:bg-transparent"
        >
          {open ? (
            <ChevronDown className="h-4 w-4 text-muted-foreground" />
          ) : (
            <ChevronRight className="h-4 w-4 text-muted-foreground" />
          )}
          <Layers3 className="h-4 w-4 text-muted-foreground" />
          {t('detail.qa.sources', { count: result.sources.length })}
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-2">
        {result.sources.map((source, index) => (
          <div
            key={source.primary_chunk_id}
            className="rounded-md border border-border bg-muted/20 p-3"
          >
            <div className="flex flex-wrap items-center gap-2 text-sm font-medium text-foreground">
              <span>#{source.position + 1}</span>
              <Badge variant="subtle">{t('detail.chunks.primary')}</Badge>
              <span className="text-xs text-muted-foreground">
                {t('detail.qa.similarityRank', { rank: index + 1 })}
              </span>
              {onLocateSource ? (
                <Button
                  type="button"
                  variant="link"
                  className="ml-auto h-auto gap-1 p-0 text-xs font-medium text-primary"
                  onClick={() => onLocateSource(source)}
                >
                  <LocateFixed className="h-3.5 w-3.5" />
                  {t('detail.qa.locateSource')}
                </Button>
              ) : null}
            </div>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">{source.snippet}</p>
          </div>
        ))}
      </CollapsibleContent>
    </Collapsible>
  );
}
