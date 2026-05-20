import React from 'react';
import { Bot, Lightbulb } from 'lucide-react';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';

export interface ChatHomeViewProps {
  className?: string;
  onSuggestionClick?: (text: string) => void;
  title?: string;
  description?: string;
  suggestions?: string[];
  showDefaultSuggestions?: boolean;
  suggestionsTitle?: string;
}

export function ChatHomeView({
  className,
  onSuggestionClick,
  title,
  description,
  suggestions,
  showDefaultSuggestions = false,
  suggestionsTitle,
}: ChatHomeViewProps) {
  const t = useT('webapp');

  const defaultTitle = t('chat.welcome');
  const resolvedTitle = title === '' ? '' : title || defaultTitle;
  const defaultSuggestions = showDefaultSuggestions
    ? [t('chat.suggestions.help'), t('chat.suggestions.summarize'), t('chat.suggestions.write')]
    : [];

  const configuredSuggestions = (suggestions ?? [])
    .map(suggestion => suggestion.trim())
    .filter(Boolean)
    .slice(0, 3);
  const displaySuggestions =
    configuredSuggestions.length > 0 ? configuredSuggestions : defaultSuggestions;
  const hasDisplaySuggestions = displaySuggestions.length > 0;

  return (
    <div
      className={cn(
        'flex h-full w-full min-w-0 flex-col items-center justify-center overflow-hidden px-4 animate-in fade-in zoom-in-95 duration-500',
        className
      )}
    >
      {resolvedTitle ? (
        <div className="mb-4 rounded-lg bg-primary/10 p-3">
          <Bot className="size-8 text-primary" />
        </div>
      ) : null}

      {resolvedTitle ? (
        <h1 className="mb-2 max-w-2xl break-words text-center text-xl font-semibold leading-7 text-foreground">
          {resolvedTitle}
        </h1>
      ) : null}

      {description && (
        <p className="mb-8 max-w-md break-words text-center leading-7 tracking-[0.02em] text-muted-foreground">
          {description}
        </p>
      )}

      {!description && resolvedTitle && <div className="h-4" />}

      {hasDisplaySuggestions && suggestionsTitle ? (
        <div className="mt-3 text-xs font-medium text-muted-foreground">{suggestionsTitle}</div>
      ) : null}

      {hasDisplaySuggestions && (
        <div
          className={cn(
            'grid w-full max-w-xl grid-cols-1 gap-2',
            suggestionsTitle ? 'mt-2' : 'mt-3'
          )}
        >
          {displaySuggestions.map((suggestion, index) => (
            <button
              key={index}
              onClick={() => onSuggestionClick?.(suggestion)}
              className="group flex items-center rounded-lg border border-border bg-background px-3 py-2.5 text-left shadow-sm transition-colors hover:border-primary/40 hover:bg-primary/5"
            >
              <div className="mr-3 rounded-md bg-muted p-1.5 transition-colors group-hover:bg-background">
                <Lightbulb className="size-4 text-muted-foreground" />
              </div>
              <span className="text-sm leading-5 text-foreground/80 line-clamp-2">
                {suggestion}
              </span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
