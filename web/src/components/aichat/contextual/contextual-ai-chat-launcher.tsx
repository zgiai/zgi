'use client';

import { Sparkles } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';
import { useContextualAIChat } from './contextual-ai-chat-context';

interface ContextualAIChatLauncherProps {
  className?: string;
  compact?: boolean;
}

export function ContextualAIChatLauncher({ className, compact }: ContextualAIChatLauncherProps) {
  const { open, items } = useContextualAIChat();
  const contextCount = items.length;

  return (
    <Button
      type="button"
      aria-label={`Open AIChat assistant with ${contextCount} context ${
        contextCount === 1 ? 'item' : 'items'
      }`}
      variant="outline"
      size={compact ? 'default' : 'sm'}
      isIcon={compact}
      className={cn(
        'relative border-primary/25 bg-primary/5 text-primary hover:bg-primary/10 hover:text-primary',
        !compact && 'gap-2',
        className
      )}
      onClick={open}
      title="Open AIChat assistant"
    >
      <Sparkles className="size-4" />
      {!compact ? <span className="hidden lg:inline">AIChat</span> : null}
      {contextCount > 0 ? (
        <Badge
          variant="info"
          className="absolute -right-2 -top-2 h-5 min-w-5 px-1 text-[10px] leading-none"
        >
          {contextCount}
        </Badge>
      ) : null}
    </Button>
  );
}
