'use client';

import { Sparkles } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import { useContextualAIChat } from './contextual-ai-chat-context';

interface ContextualAIChatLauncherProps {
  className?: string;
  compact?: boolean;
}

export function ContextualAIChatLauncher({ className, compact }: ContextualAIChatLauncherProps) {
  const { isOpen, setOpen } = useContextualAIChat();
  const t = useT('webapp');
  const assistantLabel = t('consoleChat.contextual.assistantLabel');

  return (
    <Button
      type="button"
      aria-label={assistantLabel}
      variant="outline"
      size={compact ? 'default' : 'sm'}
      isIcon={compact}
      className={cn(
        'border-primary/25 bg-primary/5 text-primary hover:bg-primary/10 hover:text-primary',
        !compact && 'gap-2',
        className
      )}
      onClick={() => setOpen(!isOpen)}
      title={assistantLabel}
      aria-pressed={isOpen}
    >
      <Sparkles className="size-4" />
      {!compact ? <span className="hidden lg:inline">{assistantLabel}</span> : null}
    </Button>
  );
}
