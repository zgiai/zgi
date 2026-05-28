import React from 'react';
import { Bot } from 'lucide-react';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { ICON_BG, ICON_TEXT } from '@/lib/config';
import { cn } from '@/lib/utils';
import { SUGGESTED_QUESTIONS_LIMIT } from '@/constants/suggested-questions';

interface ChatOpeningMessageProps {
  className?: string;
  content: string;
  title?: string;
  iconType?: 'image' | 'text' | string;
  icon?: string;
  iconBackground?: string;
  iconSrc?: string;
  suggestions?: string[];
  onSuggestionClick?: (text: string) => void;
}

/**
 * @component ChatOpeningMessage
 * @category UI
 * @status Stable
 * @description Renders a workflow-configured opening statement as an assistant-style empty state.
 * @usage Use in chat containers when a conversation has no messages and a workflow defines an opening statement.
 * @example
 * <ChatOpeningMessage title="Assistant" content="## Hello\nHow can I help today?" />
 */
export function ChatOpeningMessage({
  className,
  content,
  title,
  iconType,
  icon,
  iconBackground,
  iconSrc,
  suggestions,
  onSuggestionClick,
}: ChatOpeningMessageProps) {
  const normalizedContent = content.trim();
  const normalizedTitle = typeof title === 'string' ? title.trim() : '';
  const normalizedSuggestions = (suggestions ?? [])
    .map(suggestion => suggestion.trim())
    .filter(Boolean)
    .slice(0, SUGGESTED_QUESTIONS_LIMIT);
  const hasSuggestions = normalizedSuggestions.length > 0;

  if (!normalizedContent) {
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

      <div className="mt-4 w-full rounded-lg bg-muted/70 px-4 py-3 text-sm leading-6">
        <MarkdownViewer
          className="md-viewer max-w-full break-words [&_p]:my-0 [&_pre]:max-w-full [&_pre]:overflow-x-auto [&_table]:block [&_table]:max-w-full [&_table]:overflow-x-auto"
          content={normalizedContent}
        />
      </div>

      {hasSuggestions ? (
        <div className="mt-2 flex w-full flex-wrap gap-2">
          {normalizedSuggestions.map((suggestion, index) => (
            <button
              key={`${suggestion}-${index}`}
              type="button"
              onClick={() => onSuggestionClick?.(suggestion)}
              className="max-w-full rounded-lg border border-border bg-background px-4 py-2 text-left text-sm leading-5 text-foreground/80 shadow-sm transition-colors hover:border-primary/40 hover:bg-primary/5"
            >
              <span className="line-clamp-2 break-words">{suggestion}</span>
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}
