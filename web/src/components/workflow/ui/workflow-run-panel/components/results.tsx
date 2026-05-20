import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import JsonView from '@uiw/react-json-view';
import { lightTheme } from '@uiw/react-json-view/light';
import MarkdownViewer from '@/components/common/markdown-viewer';
import type { HistoryResult } from '../types';
import { Button } from '@/components/ui/button';
import { Check, Copy, FileText } from 'lucide-react';
import { useT } from '@/i18n';
import { useLocale } from '@/hooks/use-locale';
import { isSensitiveOutputBlockedValue } from '@/utils/model-output-filter';
import {
  QuestionAnswerTranscript,
  questionAnswerTranscriptToText,
} from '@/components/workflow/question-answer/question-answer-transcript';
import type { QuestionAnswerTranscriptItem } from '@/components/workflow/question-answer/runtime-events';

interface ResultsProps {
  mode: 'draft' | 'history';
  title?: string;
  streamedText?: string;
  historyResult?: HistoryResult | null;
  emptyText: string;
  questionAnswerTranscript?: QuestionAnswerTranscriptItem[];
}

interface FlatOutputField {
  key: string;
  label: string;
  value: string;
}

const outputFieldLabels: Record<string, { en: string; zh: string }> = {
  account_id: { en: 'Account ID', zh: '账号 ID' },
  account_name: { en: 'Account name', zh: '客户名称' },
  analysis: { en: 'Analysis', zh: '分析结果' },
  answer: { en: 'Answer', zh: '答案' },
  brief: { en: 'Brief', zh: '摘要' },
  count: { en: 'Count', zh: '数量' },
  decision: { en: 'Decision', zh: '决策建议' },
  digest: { en: 'Digest', zh: '简报' },
  image_urls: { en: 'Image URLs', zh: '图片链接' },
  normalized_json: { en: 'Normalized JSON', zh: '规范化结果' },
  output: { en: 'Output', zh: '输出' },
  priority: { en: 'Priority', zh: '优先级' },
  rejection: { en: 'Rejection', zh: '拒绝原因' },
  requires_review: { en: 'Requires review', zh: '是否需要复核' },
  review_notes: { en: 'Review notes', zh: '审核备注' },
  review_summary: { en: 'Review summary', zh: '审查摘要' },
  reviewed_draft: { en: 'Reviewed draft', zh: '复核稿' },
  revision_request: { en: 'Revision request', zh: '修改要求' },
  severity: { en: 'Severity', zh: '严重程度' },
  status: { en: 'Status', zh: '状态' },
  task_id: { en: 'Task ID', zh: '任务 ID' },
  topic: { en: 'Topic', zh: '主题' },
};

function formatOutputKey(key: string, locale: string): string {
  const localizedLabel = outputFieldLabels[key];
  if (localizedLabel) return locale.startsWith('zh') ? localizedLabel.zh : localizedLabel.en;

  const readable = key.replace(/[_-]+/g, ' ').trim();
  if (!readable) return key;
  return readable.replace(/\b[a-z]/g, match => match.toUpperCase());
}

function formatFlatOutputValue(value: unknown): string | null {
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);

  if (Array.isArray(value)) {
    const values = value.map(item => formatFlatOutputValue(item));
    if (values.every(Boolean)) return values.join('\n');
  }

  return null;
}

function getFlatOutputFields(value: unknown, locale: string): FlatOutputField[] | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null;

  const entries = Object.entries(value as Record<string, unknown>);
  if (entries.length === 0) return null;

  const fields = entries.map(([key, rawValue]) => {
    const formattedValue = formatFlatOutputValue(rawValue);
    return formattedValue === null
      ? null
      : {
          key,
          label: formatOutputKey(key, locale),
          value: formattedValue,
        };
  });

  return fields.every(Boolean) ? (fields as FlatOutputField[]) : null;
}

/**
 * Results - Render streaming or historical outputs with auto-scroll.
 */
const Results: React.FC<ResultsProps> = ({
  mode,
  title,
  streamedText,
  historyResult,
  emptyText,
  questionAnswerTranscript = [],
}) => {
  const t = useT('common');
  const tAll = useT();
  const { locale } = useLocale();
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const [copied, setCopied] = useState<boolean>(false);

  const displayText = useCallback(
    (value: string): string =>
      isSensitiveOutputBlockedValue(value) ? t('sensitiveOutput.blocked') : value,
    [t]
  );

  const transcriptText = useMemo(
    () =>
      questionAnswerTranscriptToText(questionAnswerTranscript, {
        question: tAll('nodes.questionAnswer.runtime.questionLabel'),
        answer: tAll('nodes.questionAnswer.runtime.answerLabel'),
      }),
    [questionAnswerTranscript, tAll]
  );
  const hasTranscript = transcriptText.trim().length > 0;

  const flatOutputFields = useMemo<FlatOutputField[] | null>(() => {
    if (historyResult?.kind !== 'json') return null;
    return getFlatOutputFields(historyResult.value, locale);
  }, [historyResult, locale]);

  // Determine content text to copy from current mode
  const copyText = useMemo<string>(() => {
    const body = (() => {
      if (historyResult) {
        if (historyResult.kind === 'text') return displayText(historyResult.content ?? '');
        if (historyResult.kind === 'json') {
          if (flatOutputFields) {
            return flatOutputFields
              .map(field => `${field.label}: ${displayText(field.value)}`)
              .join('\n');
          }

          try {
            return JSON.stringify((historyResult.value as unknown) ?? {}, null, 2);
          } catch {
            return '';
          }
        }
        return '';
      }
      if (mode === 'draft') return displayText(streamedText ?? '');
      return '';
    })();
    if (!transcriptText) return body;
    return body ? `${transcriptText}\n\n${body}` : transcriptText;
  }, [mode, streamedText, historyResult, displayText, transcriptText, flatOutputFields]);

  const hasContent = copyText.trim().length > 0;

  // Copy handler with graceful fallback
  const handleCopy = async () => {
    if (!hasContent) return;
    try {
      if (navigator?.clipboard?.writeText) {
        await navigator.clipboard.writeText(copyText);
      } else {
        const ta = document.createElement('textarea');
        ta.value = copyText;
        ta.style.position = 'fixed';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        document.body.removeChild(ta);
      }
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      // ignore copy errors
    }
  };

  useEffect(() => {
    if (mode !== 'draft') return;
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [mode, streamedText]);

  const renderJsonResult = (value: unknown) => {
    const fields = getFlatOutputFields(value, locale);

    if (fields) {
      return (
        <div className="space-y-2">
          {fields.map(field => (
            <div
              key={field.key}
              className="rounded-md border border-border/50 bg-background px-3 py-2"
            >
              <div className="text-xs font-medium text-muted-foreground">{field.label}</div>
              <div className="mt-1 whitespace-pre-wrap break-words text-sm leading-relaxed text-foreground">
                {displayText(field.value)}
              </div>
            </div>
          ))}
        </div>
      );
    }

    return (
      <JsonView
        value={(value as unknown) ?? {}}
        style={lightTheme}
        className="rounded-lg overflow-auto bg-muted/20 p-3 border border-border/40 font-mono text-sm leading-relaxed"
      />
    );
  };

  return (
    <div className="flex-1 flex flex-col min-h-0 bg-background/20 group relative">
      <div className="px-3 py-1.5 border-b flex items-center justify-between bg-muted/30">
        <div className="flex items-center gap-1.5 h-6">
          <FileText className="w-5 h-5 text-violet-500" />
          <span className="text-sm font-semibold">{title || t('results')}</span>
        </div>
        <div className="hidden group-hover:block transition-all duration-200 h-6 w-6">
          <Button
            type="button"
            variant="ghost"
            isIcon
            className="h-6 w-6"
            aria-label={t('copyOutput')}
            disabled={!hasContent}
            onClick={handleCopy}
          >
            {copied ? <Check className="h-4 w-4 text-emerald-500" /> : <Copy className="h-4 w-4" />}
          </Button>
        </div>
      </div>

      <div ref={scrollRef} className="flex-1 overflow-auto p-3 scrollbar-thin">
        {mode === 'draft' ? (
          <>
            {hasTranscript ? (
              <QuestionAnswerTranscript items={questionAnswerTranscript} className="mb-3" />
            ) : null}
            {historyResult ? (
              historyResult.kind === 'text' ? (
                <MarkdownViewer content={displayText(historyResult.content)} />
              ) : historyResult.kind === 'json' ? (
                renderJsonResult(historyResult.value)
              ) : (
                <div className="h-full flex flex-col items-center justify-center gap-4 py-12 text-muted-foreground/50">
                  <FileText className="w-12 h-12 stroke-[1.5] shrink-0" />
                  <span className="text-sm font-medium">{emptyText}</span>
                </div>
              )
            ) : streamedText && streamedText.length > 0 ? (
              <MarkdownViewer content={displayText(streamedText)} />
            ) : !hasTranscript ? (
              <div className="h-full flex flex-col items-center justify-center gap-4 py-12 text-muted-foreground/40">
                <FileText className="w-12 h-12 stroke-[1.5] shrink-0" />
                <span className="text-sm font-medium">{emptyText}</span>
              </div>
            ) : null}
          </>
        ) : historyResult && historyResult.kind === 'text' ? (
          <MarkdownViewer content={displayText(historyResult.content)} />
        ) : historyResult && historyResult.kind === 'json' ? (
          renderJsonResult(historyResult.value)
        ) : (
          <div className="h-full flex flex-col items-center justify-center gap-4 py-12 text-muted-foreground/40">
            <FileText className="w-12 h-12 stroke-[1.5] shrink-0" />
            <span className="text-sm font-medium">{emptyText}</span>
          </div>
        )}
      </div>
    </div>
  );
};

export default Results;
