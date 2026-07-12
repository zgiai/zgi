'use client';

import type { ReactNode } from 'react';
import { History, MessageSquarePlus } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

interface AIChatEmbeddedConversationControlsProps {
  openConversations: () => void;
  startNewConversation: () => void;
  conversationsLabel: string;
  newConversationLabel: string;
  trailingAction?: ReactNode;
  className?: string;
}

const embeddedControlButtonClassName =
  'size-7 rounded-[4px] bg-muted/40 text-muted-foreground hover:bg-muted hover:text-foreground';

export function AIChatEmbeddedConversationControls({
  openConversations,
  startNewConversation,
  conversationsLabel,
  newConversationLabel,
  trailingAction,
  className,
}: AIChatEmbeddedConversationControlsProps) {
  return (
    <div className={cn('flex items-center gap-1', className)}>
      <Button
        type="button"
        variant="ghost"
        isIcon
        className={embeddedControlButtonClassName}
        onClick={openConversations}
        title={conversationsLabel}
        aria-label={conversationsLabel}
      >
        <History className="size-3.5" />
      </Button>
      <Button
        type="button"
        variant="ghost"
        isIcon
        className={embeddedControlButtonClassName}
        onClick={startNewConversation}
        title={newConversationLabel}
        aria-label={newConversationLabel}
      >
        <MessageSquarePlus className="size-3.5" />
      </Button>
      {trailingAction}
    </div>
  );
}

export { embeddedControlButtonClassName };
