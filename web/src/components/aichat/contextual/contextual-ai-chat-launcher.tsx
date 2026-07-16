'use client';

import { Button } from '@/components/ui/button';
import { AIChatBrandMark } from '@/components/chat/variants/aichat/brand-mark';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import { useContextualAIChat } from './contextual-ai-chat-context';

interface ContextualAIChatLauncherProps {
  className?: string;
  compact?: boolean;
}

export function ContextualAIChatLauncher({ className, compact }: ContextualAIChatLauncherProps) {
  const { isAvailable, isOpen, setOpen } = useContextualAIChat();
  const t = useT('webapp');
  const assistantLabel = t('consoleChat.contextual.assistantLabel');

  if (!isAvailable) return null;

  return (
    <Button
      type="button"
      aria-label={assistantLabel}
      variant="outline"
      size={compact ? 'default' : 'sm'}
      isIcon={compact}
      className={cn(
        'hidden border-primary/25 bg-primary/5 text-primary hover:bg-primary/10 hover:text-primary lg:inline-flex',
        !compact && 'gap-2',
        className
      )}
      onClick={() => setOpen(!isOpen)}
      title={assistantLabel}
      aria-pressed={isOpen}
    >
      <AIChatBrandMark variant="compact" />
      {!compact ? <span className="hidden lg:inline">{assistantLabel}</span> : null}
    </Button>
  );
}
