'use client';

import * as React from 'react';
import { Button } from '@/components/ui/button';
import { ChatOpeningGuideView } from '@/components/chat/ui/chat-opening-guide-view';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import type { AIChatSuggestion } from '@/components/chat/variants/aichat/types';
import type { OpeningGuideBrand } from '@/components/chat/utils/opening-guide-brand';
import { AIChatBrandMark } from './brand-mark';

interface AIChatHomeViewProps {
  isVisible: boolean;
  suggestions: AIChatSuggestion[];
  onSelectSuggestion: (value: string) => void;
  brand?: React.ReactNode;
  openingGuideBrand?: OpeningGuideBrand;
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
  openingGuideBrand,
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

  if (surface !== 'aichat') {
    return (
      <div
        className={cn(
          'absolute inset-0 z-0 px-4 transition-all duration-300 ease-in-out',
          isVisible ? 'scale-100 opacity-100' : 'pointer-events-none -z-10 scale-95 opacity-0'
        )}
      >
        <div
          className={cn('h-full overflow-y-auto', surface === 'agent-webapp' ? 'pt-20' : 'pt-4')}
          style={{ paddingBottom: composerHeightPx + 24 }}
        >
          <div className="mx-auto flex min-h-full w-full min-w-0 max-w-6xl flex-col items-center justify-center overflow-hidden px-4 py-8">
            <ChatOpeningGuideView
              title={title || t('chat.startConversation')}
              message={resolvedDescription}
              iconType={openingGuideBrand?.iconType}
              icon={openingGuideBrand?.icon}
              iconBackground={openingGuideBrand?.iconBackground}
              iconSrc={openingGuideBrand?.iconSrc}
              suggestions={suggestions.map(suggestion => suggestion.text)}
              onSuggestionClick={onSelectSuggestion}
            />
          </div>
        </div>
      </div>
    );
  }

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
          className="flex w-full max-w-3xl flex-col items-center gap-4"
          style={{
            transform:
              'translateY(calc(-100% - var(--aichat-home-composer-half) - var(--aichat-home-title-gap)))',
          }}
        >
          {brand ? brand : isHydrated ? <AIChatBrandMark /> : null}
          <h2 className="text-2xl font-bold text-foreground">
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
          className="flex w-full max-w-3xl flex-wrap items-center justify-center gap-2"
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
