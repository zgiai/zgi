'use client';

import { Menu, PanelLeft, Plus } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';

interface AIChatHeaderProps {
  isMobile: boolean;
  isHome: boolean;
  title: string;
  onToggleSidebar: () => void;
  onStartNew: () => void;
}

/**
 * @component AIChatHeader
 * @category Feature
 * @status Stable
 * @description Floating console chat header with sidebar and new-chat actions.
 * @usage Render at the top of AIChatShell main area
 * @example
 * <AIChatHeader isMobile={false} isHome={false} title="Chat" onToggleSidebar={toggle} onStartNew={start} />
 */
export function AIChatHeader({
  isMobile,
  isHome,
  title,
  onToggleSidebar,
  onStartNew,
}: AIChatHeaderProps) {
  const t = useT('webapp');

  return (
    <header className="absolute left-0 right-0 top-0 z-20 flex h-12 items-center justify-between bg-background/80 px-3 backdrop-blur-sm transition-opacity duration-300 sm:h-14 sm:px-4">
      <div className="flex items-center gap-2">
        <Button
          variant="ghost"
          isIcon
          className="size-8 text-muted-foreground"
          onClick={onToggleSidebar}
          title={t('consoleChat.toggleSidebar')}
        >
          {isMobile ? <Menu className="size-4" /> : <PanelLeft className="size-4" />}
        </Button>
        {!isHome ? (
          <Button
            variant="ghost"
            isIcon
            className="size-8 text-muted-foreground"
            onClick={onStartNew}
            title={t('chat.newConversation')}
          >
            <Plus className="size-4" />
          </Button>
        ) : null}
      </div>
      <div
        className={cn(
          'min-w-0 px-3 text-center transition-opacity duration-300',
          isHome ? 'opacity-0' : 'opacity-100'
        )}
      >
        <h1 className="truncate text-sm font-semibold">{title}</h1>
      </div>
      <div className="w-20" />
    </header>
  );
}
