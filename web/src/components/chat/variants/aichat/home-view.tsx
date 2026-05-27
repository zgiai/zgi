'use client';

import * as React from 'react';
import { Button } from '@/components/ui/button';
import { ZgiDrawingWordmark } from '@/components/brand/zgi-drawing-wordmark';
import { useT } from '@/i18n/translations';
import { APP_NAME, HAS_CUSTOM_LOGO_URL, ICON_TEXT, IS_ZGI_BRAND, LOGO_URL } from '@/lib/config';
import { cn } from '@/lib/utils';
import type { AIChatSuggestion } from '@/components/chat/variants/aichat/types';

interface AIChatHomeViewProps {
  isVisible: boolean;
  suggestions: AIChatSuggestion[];
  onSelectSuggestion: (value: string) => void;
  brand?: React.ReactNode;
  title?: string;
  description?: string;
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
}: AIChatHomeViewProps) {
  const t = useT('webapp');
  const [isHydrated, setIsHydrated] = React.useState(false);
  const fallbackText = (ICON_TEXT || APP_NAME.charAt(0) || 'A').slice(0, 2).toUpperCase();
  const resolvedDescription =
    description === '' ? '' : description || t('chat.chooseAssistant');

  React.useEffect(() => {
    setIsHydrated(true);
  }, []);

  return (
    <div
      className={cn(
        'absolute inset-0 z-0 flex items-center justify-center px-4 text-center transition-all duration-300 ease-in-out',
        isVisible ? 'scale-100 opacity-100' : 'pointer-events-none -z-10 scale-95 opacity-0'
      )}
    >
      <div className="flex w-full max-w-3xl animate-in flex-col items-center gap-8 -mt-20 duration-500 fade-in zoom-in">
        <div className="flex flex-col items-center gap-4">
          {brand ? (
            brand
          ) : isHydrated ? (
            <div className="flex size-16 items-center justify-center rounded-2xl border border-brand-main/50 bg-bg-surface shadow-[0_0_0_4px_rgba(0,75,255,0.04),0_8px_18px_rgba(10,11,13,0.06)]">
              {IS_ZGI_BRAND ? (
                <ZgiDrawingWordmark className="w-11" loop={false} />
              ) : HAS_CUSTOM_LOGO_URL ? (
                <img src={LOGO_URL} alt={APP_NAME} className="max-h-10 max-w-11 object-contain" />
              ) : (
                <span className="text-lg font-semibold text-brand-main">{fallbackText}</span>
              )}
            </div>
          ) : null}
          <h2 className="text-2xl font-bold text-foreground">
            {title || t('chat.startConversation')}
          </h2>
          {resolvedDescription ? (
            <p className="text-sm text-muted-foreground">{resolvedDescription}</p>
          ) : null}
        </div>
        <div className="h-[140px] w-full shrink-0" />
        <div className="flex flex-wrap items-center justify-center gap-2">
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
