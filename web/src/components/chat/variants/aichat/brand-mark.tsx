import { ZgiDrawingWordmark } from '@/components/brand/zgi-drawing-wordmark';
import { APP_NAME, HAS_CUSTOM_LOGO_URL, ICON_TEXT, IS_ZGI_BRAND, LOGO_URL } from '@/lib/config';
import { cn } from '@/lib/utils';

interface AIChatBrandMarkProps {
  className?: string;
  variant?: 'home' | 'compact';
}

export function AIChatBrandMark({ className, variant = 'home' }: AIChatBrandMarkProps) {
  const fallbackText = (ICON_TEXT || APP_NAME.charAt(0) || 'A').slice(0, 2).toUpperCase();
  const compact = variant === 'compact';

  return (
    <span
      className={cn(
        'inline-flex shrink-0 items-center justify-center overflow-hidden',
        compact
          ? 'size-5 rounded-md'
          : 'size-16 rounded-2xl border border-brand-main/50 bg-bg-surface shadow-[0_0_0_4px_rgba(0,75,255,0.04),0_8px_18px_rgba(10,11,13,0.06)]',
        className
      )}
    >
      {IS_ZGI_BRAND ? (
        <ZgiDrawingWordmark className={compact ? 'w-5' : 'w-11'} loop={false} />
      ) : HAS_CUSTOM_LOGO_URL ? (
        <img
          src={LOGO_URL}
          alt={APP_NAME}
          className={
            compact ? 'max-h-5 max-w-5 object-contain' : 'max-h-10 max-w-11 object-contain'
          }
        />
      ) : (
        <span
          className={cn(
            'font-semibold text-brand-main',
            compact ? 'text-[10px] leading-none' : 'text-lg'
          )}
        >
          {fallbackText}
        </span>
      )}
    </span>
  );
}
