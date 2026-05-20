import React from 'react';
import { HelpCircle, Loader2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import type { QuestionAnswerChoice } from '@/services/types/workflow';

export interface QuestionAnswerRuntimePromptProps {
  question: string;
  choices?: QuestionAnswerChoice[];
  round?: number;
  submitting?: boolean;
  compact?: boolean;
  onSelectChoice?: (choice: QuestionAnswerChoice) => void;
}

function choiceText(choice: QuestionAnswerChoice): string {
  return String(choice.label || choice.value || choice.id || '').trim();
}

export function QuestionAnswerRuntimePrompt({
  question,
  choices = [],
  round,
  submitting = false,
  compact = false,
  onSelectChoice,
}: QuestionAnswerRuntimePromptProps) {
  const t = useT();
  const hasChoices = choices.length > 0;

  return (
    <div
      className={cn(
        'rounded-lg border bg-card text-card-foreground shadow-sm',
        compact ? 'p-3' : 'p-4'
      )}
    >
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
          <HelpCircle className="size-4" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-foreground">
              {t('nodes.questionAnswer.runtime.waitingAnswer')}
            </span>
            {typeof round === 'number' ? (
              <span className="rounded-full border bg-muted/40 px-2 py-0.5 text-[11px] text-muted-foreground">
                {round}
              </span>
            ) : null}
          </div>
          <p className="mt-1 whitespace-pre-wrap break-words text-sm leading-6 text-foreground">
            {question}
          </p>
          {hasChoices ? (
            <div className="mt-3">
              <div className="mb-2 text-xs text-muted-foreground">
                {t('nodes.questionAnswer.runtime.chooseOne')}
              </div>
              <div className="flex flex-wrap gap-2">
                {choices.map(choice => {
                  const text = choiceText(choice);
                  return (
                    <Button
                      key={choice.id}
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={submitting}
                      className="h-8 max-w-full justify-start rounded-md"
                      onClick={() => onSelectChoice?.(choice)}
                      title={text}
                    >
                      {submitting ? <Loader2 className="mr-1.5 size-3.5 animate-spin" /> : null}
                      <span className="truncate">{text || choice.id}</span>
                    </Button>
                  );
                })}
              </div>
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}

export function getQuestionAnswerChoiceQuery(choice: QuestionAnswerChoice): string {
  return choiceText(choice) || String(choice.id || '').trim();
}
