import React, { useCallback, useEffect, useRef } from 'react';
import type { ChatController } from '@/components/chat/controllers/types';
import { Button } from '@/components/ui/button';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';
import { Plus, Trash2 } from 'lucide-react';
import { useT } from '@/i18n';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { useStore } from 'zustand';
import { toast } from 'sonner';
import { useIsMobile } from '@/hooks/use-mobile';

function getSidebarBackgroundStyle(backgroundImage?: string): React.CSSProperties | undefined {
  if (!backgroundImage) return undefined;

  return {
    backgroundImage: `url(${JSON.stringify(backgroundImage)})`,
    backgroundPosition: 'top center',
    backgroundRepeat: 'repeat-y',
    backgroundSize: '100% auto',
  };
}

interface ConversationHistoryListProps {
  controller: ChatController;
  className?: string;
  backgroundImage?: string;
  onCreateWhileDraft?: () => void;
  onActionComplete?: () => void;
}

const ConversationHistoryList: React.FC<ConversationHistoryListProps> = ({
  controller,
  className,
  backgroundImage,
  onCreateWhileDraft,
  onActionComplete,
}) => {
  const activeId = useStore(controller.store, s => s.activeId);
  const conversations = useStore(controller.store, s => s.conversations);
  const pagination = useStore(controller.store, s => s.pagination);
  const isLoadingList = useStore(controller.store, s => s.isLoadingList);
  const isMobile = useIsMobile();
  const t = useT();
  const activeConversation = conversations.find(item => item.id === activeId);
  const isActiveDraft =
    !!activeConversation &&
    (!activeConversation.conversationId || activeConversation.id.startsWith('draft-'));

  const handleCreate = useCallback(() => {
    if (isActiveDraft) {
      toast.info(t('webapp.chat.alreadyInDraft'));
      onCreateWhileDraft?.();
      onActionComplete?.();
      return;
    }
    const draft = controller.createDraft(t('webapp.chat.newConversation'));
    controller.select(draft.id);
    onActionComplete?.();
  }, [controller, isActiveDraft, onActionComplete, onCreateWhileDraft, t]);

  const handleSelect = useCallback(
    (id: string) => {
      controller.select(id);
      onActionComplete?.();
    },
    [controller, onActionComplete]
  );

  const handleRemove = useCallback(
    (id: string) => {
      controller.remove(id);
    },
    [controller]
  );
  const renderEmptyState = () => (
    <div className="rounded-lg border border-dashed bg-muted/30 px-3 py-6 text-center">
      <div className="text-sm font-medium text-foreground">{t('webapp.chat.noHistory')}</div>
      <div className="mt-1 text-xs leading-5 text-muted-foreground">
        {t('webapp.chat.startNewChat')}
      </div>
    </div>
  );
  const getDisplayTitle = useCallback(
    (title?: string) => {
      const trimmedTitle = title?.trim();
      if (!trimmedTitle) return t('webapp.chat.untitledConversation');
      const defaultTitleMatch = trimmedTitle.match(/^Conversation\s+(\d{4}-\d{2}-\d{2}.*)$/);
      if (defaultTitleMatch?.[1]) {
        return `${t('webapp.chat.conversationPrefix')} ${defaultTitleMatch[1]}`;
      }
      return trimmedTitle;
    },
    [t]
  );

  const viewportRef = useRef<HTMLDivElement>(null);
  const pendingPageRef = useRef<number | null>(null);

  useEffect(() => {
    const pendingPage = pendingPageRef.current;
    if (pendingPage === null) return;

    if (!pagination.hasMore || pagination.page >= pendingPage) {
      pendingPageRef.current = null;
    }
  }, [pagination.hasMore, pagination.page]);

  const handleScroll = useCallback(
    (e: React.UIEvent<HTMLDivElement>) => {
      const el = e.currentTarget;
      const threshold = 64;
      const nearBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - threshold;

      if (!nearBottom) {
        pendingPageRef.current = null;
        return;
      }

      const nextPage = pagination.page + 1;
      if (!pagination.hasMore || isLoadingList || pendingPageRef.current === nextPage) {
        return;
      }

      pendingPageRef.current = nextPage;
      void controller.refreshList({
        page: nextPage,
        limit: pagination.limit,
        append: true,
      });
    },
    [controller, pagination.page, pagination.limit, pagination.hasMore, isLoadingList]
  );
  const backgroundStyle = getSidebarBackgroundStyle(backgroundImage);
  const renderConversationItems = () => (
    <>
      {conversations.length === 0 ? renderEmptyState() : null}
      {conversations.map(item => {
        const displayTitle = getDisplayTitle(item.title);
        return (
          <div
            key={item.id}
            className={cn(
              'group flex cursor-pointer items-center justify-between rounded-md px-2 py-2 text-sm hover:bg-muted',
              activeId === item.id && 'bg-accent'
            )}
            onClick={() => handleSelect(item.id)}
          >
            <div className="w-0 grow pr-2">
              <div className="truncate font-medium" title={displayTitle}>
                {displayTitle}
              </div>
              <div className="truncate text-xs text-muted-foreground">
                {t('webapp.chat.messageCount', { count: item.dialogueCount ?? 0 })}
              </div>
            </div>
            <div onClick={e => e.stopPropagation()}>
              <ConfirmDialog
                variant="danger"
                title={t('webapp.chat.deleteTitle')}
                description={t('webapp.chat.deleteDescription', {
                  conversationTitle: displayTitle,
                })}
                confirmText={t('common.delete')}
                cancelText={t('common.cancel')}
                onConfirm={() => handleRemove(item.id)}
                trigger={
                  <Button
                    isIcon
                    variant="ghost"
                    className="md:opacity-0 group-hover:opacity-100 hover:bg-destructive/10 hover:text-destructive"
                    onClick={e => {
                      e.stopPropagation();
                    }}
                    aria-label={t('webapp.chat.deleteConversation')}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                }
              />
            </div>
          </div>
        );
      })}
      {pagination.hasMore ? (
        <div className="py-2">{isLoadingList ? <Skeleton className="h-9 w-full" /> : null}</div>
      ) : null}
    </>
  );

  return (
    <div
      className={cn(
        'flex h-full min-h-0 w-[280px] shrink-0 flex-col overflow-hidden border-r',
        'bg-background',
        className
      )}
      style={backgroundStyle}
    >
      <div className="shrink-0 space-y-2 px-2 py-1 md:pt-3">
        <div className="text-lg font-bold">{t('webapp.chat.conversations')}</div>
        <Button size="sm" className="w-full font-normal" onClick={handleCreate}>
          <Plus className="h-4 w-4" />
          {t('webapp.chat.newConversation')}
        </Button>
      </div>
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        {isLoadingList && conversations.length === 0 ? (
          <div className="p-3 space-y-2">
            <Skeleton className="h-9 w-full" />
            <Skeleton className="h-9 w-full" />
            <Skeleton className="h-9 w-full" />
            <Skeleton className="h-9 w-full" />
          </div>
        ) : isMobile ? (
          <div
            ref={viewportRef}
            className="min-h-0 flex-1 overflow-y-auto overscroll-contain"
            onScroll={handleScroll}
          >
            <div className="p-2 space-y-1">{renderConversationItems()}</div>
          </div>
        ) : (
          <ScrollArea
            className="min-h-0 flex-1 w-full"
            viewportRef={viewportRef}
            viewportProps={{ onScroll: handleScroll }}
          >
            <div className="p-2 space-y-1">{renderConversationItems()}</div>
          </ScrollArea>
        )}
      </div>
    </div>
  );
};

export default ConversationHistoryList;
