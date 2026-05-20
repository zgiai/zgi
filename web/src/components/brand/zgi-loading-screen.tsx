'use client';

import { APP_NAME, HAS_CUSTOM_LOGO_URL, ICON_TEXT, IS_ZGI_BRAND, LOGO_URL } from '@/lib/config';
import { ZgiDrawingWordmark } from './zgi-drawing-wordmark';

type LoadingPhase = 'setup' | 'auth' | 'routing';

interface ZgiLoadingScreenProps {
  phase?: LoadingPhase;
}

export function ZgiLoadingScreen({ phase: _phase = 'setup' }: ZgiLoadingScreenProps) {
  const fallbackText = (ICON_TEXT || APP_NAME.charAt(0) || 'A').slice(0, 2).toUpperCase();

  return (
    <main
      aria-live="polite"
      aria-busy="true"
      className="flex min-h-svh items-center justify-center bg-background"
    >
      {IS_ZGI_BRAND ? (
        <ZgiDrawingWordmark className="w-[92px] sm:w-[108px]" />
      ) : HAS_CUSTOM_LOGO_URL ? (
        <img src={LOGO_URL} alt={APP_NAME} className="max-h-14 max-w-44 object-contain" />
      ) : (
        <div className="flex size-14 items-center justify-center rounded-2xl bg-primary text-xl font-semibold text-primary-foreground shadow-sm">
          {fallbackText}
        </div>
      )}
      <span className="sr-only">Loading {APP_NAME}</span>
    </main>
  );
}
