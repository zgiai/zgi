import React from 'react';
import { HelpCircle } from 'lucide-react';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { QuestionAnswerTranscriptItem } from './runtime-events';

export function normalizeQuestionAnswerTranscript(value: unknown): QuestionAnswerTranscriptItem[] {
  if (!Array.isArray(value)) return [];
  return value.reduce<QuestionAnswerTranscriptItem[]>((items, item, index) => {
    if (!item || typeof item !== 'object') return items;
    const record = item as Record<string, unknown>;
    const question = typeof record.question === 'string' ? record.question.trim() : '';
    const answer = typeof record.answer === 'string' ? record.answer.trim() : '';
    if (!question && !answer) return items;

    const next: QuestionAnswerTranscriptItem = {
      key:
        typeof record.key === 'string' && record.key.trim()
          ? record.key.trim()
          : `question-answer-${index}`,
      question,
    };
    if (typeof record.nodeId === 'string') next.nodeId = record.nodeId;
    if (typeof record.round === 'number') next.round = record.round;
    if (answer) next.answer = answer;
    items.push(next);
    return items;
  }, []);
}

export function questionAnswerTranscriptToText(
  items: QuestionAnswerTranscriptItem[],
  labels: { question: string; answer: string }
): string {
  return items
    .map(item => {
      const lines: string[] = [];
      if (item.question) lines.push(`${labels.question}: ${item.question}`);
      if (item.answer) lines.push(`${labels.answer}: ${item.answer}`);
      return lines.join('\n');
    })
    .filter(Boolean)
    .join('\n\n');
}

interface QuestionAnswerTranscriptProps {
  items: QuestionAnswerTranscriptItem[];
  className?: string;
}

export function QuestionAnswerTranscript({ items, className }: QuestionAnswerTranscriptProps) {
  const t = useT();
  const normalized = normalizeQuestionAnswerTranscript(items);
  if (normalized.length === 0) return null;

  return (
    <div className={cn('rounded-lg border bg-muted/20 p-3 text-sm', className)}>
      <div className="mb-2 flex items-center gap-2 text-xs font-medium text-muted-foreground">
        <HelpCircle className="size-3.5" />
        <span>{t('nodes.questionAnswer.runtime.transcriptTitle')}</span>
      </div>
      <div className="space-y-3">
        {normalized.map(item => (
          <div key={item.key} className="space-y-1.5">
            {item.question ? (
              <div className="grid grid-cols-[auto,1fr] gap-2">
                <span className="text-muted-foreground">
                  {t('nodes.questionAnswer.runtime.questionLabel')}
                </span>
                <span className="whitespace-pre-wrap break-words text-foreground">
                  {item.question}
                </span>
              </div>
            ) : null}
            {item.answer ? (
              <div className="grid grid-cols-[auto,1fr] gap-2">
                <span className="text-muted-foreground">
                  {t('nodes.questionAnswer.runtime.answerLabel')}
                </span>
                <span className="whitespace-pre-wrap break-words text-foreground">
                  {item.answer}
                </span>
              </div>
            ) : null}
          </div>
        ))}
      </div>
    </div>
  );
}
