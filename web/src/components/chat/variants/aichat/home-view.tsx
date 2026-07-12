'use client';

import * as React from 'react';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import type { AIChatSuggestion } from '@/components/chat/variants/aichat/types';
import { AIChatBrandMark } from './brand-mark';

interface AIChatHomeViewProps {
  isVisible: boolean;
  suggestions: AIChatSuggestion[];
  onSelectSuggestion: (value: string) => void;
  brand?: React.ReactNode;
  title?: string;
  description?: string;
  composerHeight?: number;
  surface?: 'aichat' | 'agent-draft' | 'agent-webapp';
}

/**
 * @component AIChatHomeView
 * @category Feature
 * @status Stable
 * @description Empty state for the AIChat console with prompt suggestions.
 * @usage Render as an overlay while no conversation is active
 * @example
 * <AIChatHomeView isVisible suggestions={suggestions} onSelectSuggestion={setInput} />
 */
export function AIChatHomeView({
  isVisible,
  suggestions,
  onSelectSuggestion,
  brand,
  title,
  description,
  composerHeight,
  surface = 'aichat',
}: AIChatHomeViewProps) {
  const t = useT('webapp');
  const [isHydrated, setIsHydrated] = React.useState(false);
  const resolvedDescription = description === '' ? '' : description || t('chat.chooseAssistant');
  const composerHeightPx = Math.max(96, Math.ceil(composerHeight ?? 140));
  const anchorStyle = {
    '--aichat-home-composer-half': `${Math.round(composerHeightPx / 2)}px`,
    '--aichat-home-title-gap': 'clamp(48px, 9vh, 96px)',
    '--aichat-home-suggestions-gap': '8px',
  } as React.CSSProperties;

  React.useEffect(() => {
    setIsHydrated(true);
  }, []);

  return (
    <div
      className={cn(
        'absolute inset-0 z-0 px-4 text-center transition-all duration-300 ease-in-out',
        isVisible ? 'scale-100 opacity-100' : 'pointer-events-none -z-10 scale-95 opacity-0'
      )}
      style={anchorStyle}
    >
      <div
        className={cn(
          'absolute inset-x-4 top-[58%] flex justify-center sm:top-1/2',
          'animate-in duration-500 fade-in zoom-in'
        )}
      >
        <div
          className={cn(
            'flex w-full flex-col items-center gap-4',
            surface === 'agent-draft' ? 'max-w-[560px]' : 'max-w-3xl'
          )}
          style={{
            transform:
              'translateY(calc(-100% - var(--aichat-home-composer-half) - var(--aichat-home-title-gap)))',
          }}
        >
          {brand ? brand : isHydrated ? <AIChatBrandMark /> : null}
          <h2
            className={cn(
              'font-bold text-foreground',
              surface === 'agent-draft' ? 'text-xl xl:text-2xl' : 'text-2xl'
            )}
          >
            {title || t('chat.startConversation')}
          </h2>
          {resolvedDescription ? (
            <p className="text-sm text-muted-foreground">{resolvedDescription}</p>
          ) : null}
        </div>
      </div>
      <div
        className={cn(
          'absolute inset-x-4 top-[58%] flex justify-center sm:top-1/2',
          'animate-in duration-500 fade-in zoom-in'
        )}
      >
        <div
          className={cn(
            'flex w-full flex-wrap items-center justify-center gap-2',
            surface === 'agent-draft' ? 'max-w-[560px]' : 'max-w-3xl'
          )}
          style={{
            transform:
              'translateY(calc(var(--aichat-home-composer-half) + var(--aichat-home-suggestions-gap)))',
          }}
        >
          {suggestions.map(suggestion => (
            <Button
              key={suggestion.key}
              variant="outline"
              className="h-8 rounded-full border-transparent bg-muted/30 text-xs font-normal transition-all hover:border-border/50 hover:bg-muted/50"
              onClick={() => onSelectSuggestion(suggestion.text)}
            >
              {suggestion.text}
            </Button>
          ))}
        </div>
      </div>
    </div>
  );
}
