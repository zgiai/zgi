import React from 'react';
import { Bot, Lightbulb } from 'lucide-react';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { ICON_BG, ICON_TEXT } from '@/lib/config';
import { cn } from '@/lib/utils';
import { SUGGESTED_QUESTIONS_LIMIT } from '@/constants/suggested-questions';

export interface ChatOpeningGuideViewProps {
  className?: string;
  title?: string;
  message?: string;
  iconType?: 'image' | 'text' | string;
  icon?: string;
  iconBackground?: string;
  iconSrc?: string;
  suggestions?: string[];
  onSuggestionClick?: (text: string) => void;
}

export function ChatOpeningGuideView({
  className,
  title,
  message,
  iconType,
  icon,
  iconBackground,
  iconSrc,
  suggestions,
  onSuggestionClick,
}: ChatOpeningGuideViewProps) {
  const normalizedTitle = typeof title === 'string' ? title.trim() : '';
  const normalizedMessage = typeof message === 'string' ? message.trim() : '';
  const normalizedSuggestions = (suggestions ?? [])
    .map(suggestion => suggestion.trim())
    .filter(Boolean)
    .slice(0, SUGGESTED_QUESTIONS_LIMIT);
  const hasSuggestions = normalizedSuggestions.length > 0;

  if (!normalizedTitle && !normalizedMessage && !hasSuggestions) {
    return null;
  }

  return (
    <div
      className={cn(
        'mx-auto flex w-full max-w-[548px] min-w-0 flex-col items-center overflow-hidden text-foreground',
        className
      )}
    >
      <div className="flex flex-col items-center gap-3">
        {iconType === 'image' || iconType === 'text' ? (
          <IconPreview
            iconType={iconType === 'image' ? 'image' : 'text'}
            src={iconType === 'image' ? iconSrc : undefined}
            icon={icon || ICON_TEXT}
            iconBackground={iconBackground || ICON_BG}
            alt={normalizedTitle || icon || ICON_TEXT}
            size="sm"
            editable={false}
            className="rounded-2xl"
          />
        ) : (
          <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-primary text-primary-foreground shadow-sm">
            <Bot size={24} />
          </div>
        )}
        {normalizedTitle ? (
          <h1 className="max-w-full break-words text-center text-xl font-semibold leading-7">
            {normalizedTitle}
          </h1>
        ) : null}
      </div>

      {normalizedMessage ? (
        <div className="mt-4 w-full rounded-lg bg-muted/70 px-4 py-3 text-sm leading-6">
          <MarkdownViewer
            className="md-viewer max-w-full break-words [&_p]:my-0 [&_pre]:max-w-full [&_pre]:overflow-x-auto [&_table]:block [&_table]:max-w-full [&_table]:overflow-x-auto"
            content={normalizedMessage}
          />
        </div>
      ) : null}

      {hasSuggestions ? (
        <div className="mt-3 grid w-full max-w-xl grid-cols-1 gap-2">
          {normalizedSuggestions.map((suggestion, index) => (
            <button
              key={`${suggestion}-${index}`}
              type="button"
              onClick={() => onSuggestionClick?.(suggestion)}
              className="group flex items-center rounded-lg border border-border bg-background px-3 py-2.5 text-left shadow-sm transition-colors hover:border-primary/40 hover:bg-primary/5"
            >
              <div className="mr-3 rounded-md bg-muted p-1.5 transition-colors group-hover:bg-background">
                <Lightbulb className="size-4 text-muted-foreground" />
              </div>
              <span className="line-clamp-2 text-sm leading-5 text-foreground/80">
                {suggestion}
              </span>
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}
