'use client';

import * as React from 'react';
import {
  Check,
  ArrowLeft,
  History,
  Loader2,
  MessageSquarePlus,
  Pencil,
  Search,
  Trash2,
  X,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { ScrollArea } from '@/components/ui/scroll-area';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { ConversationSearchDialog } from '@/components/chat/variants/common/conversation-search-dialog';
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from '@/components/ui/context-menu';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n/translations';
import type {
  ConversationSearchFn,
  ConversationSearchResult,
  ConversationSummary,
} from '@/components/chat/controllers/types';
import { groupByConversationTime } from '@/components/chat/variants/common/conversation-time-groups';

interface SidebarPagination {
  page: number;
  hasMore: boolean;
}

function isConversationRunning(conversation: ConversationSummary): boolean {
  const metadata = conversation.metadata;
  return Boolean(
    metadata?.runtime_status === 'streaming' ||
      metadata?.isRecovering === true ||
      metadata?.isStopping === true
  );
}

function toCssBackgroundImageUrl(url: string): string {
  return `url(${JSON.stringify(url)})`;
}

function getSidebarBackgroundStyle(backgroundImage?: string): React.CSSProperties | undefined {
  if (!backgroundImage) return undefined;

  return {
    backgroundImage: toCssBackgroundImageUrl(backgroundImage),
    backgroundPosition: 'top center',
    backgroundRepeat: 'repeat-y',
    backgroundSize: '100% auto',
  };
}

interface SidebarProps {
  isOpen: boolean;
  activeId: string | null;
  conversations: ConversationSummary[];
  onNewChat: () => void;
  onSelect: (id: string) => void;
  onDelete: (id: string) => void;
  onRename?: (id: string, title: string) => Promise<void>;
  /** Whether the chat is currently in the "new chat" (home) state */
  isHome?: boolean;
  className?: string;
  backgroundImage?: string;
  onClose?: () => void;
  alwaysShowHeader?: boolean;
  useBackNavigation?: boolean;
  search?: ConversationSearchFn;
  searchKey?: readonly unknown[];
  onSelectSearchResult?: (result: ConversationSearchResult) => void;
  pagination?: SidebarPagination;
  isLoadingList?: boolean;
  onLoadMore?: (page: number) => void | Promise<void>;
}

export function Sidebar({
  isOpen,
  activeId,
  conversations,
  onNewChat,
  onSelect,
  onDelete,
  onRename,
  className,
  backgroundImage,
  onClose,
  alwaysShowHeader = false,
  useBackNavigation = false,
  search,
  searchKey,
  onSelectSearchResult,
  pagination,
  isLoadingList = false,
  onLoadMore,
}: SidebarProps) {
  const t = useT();
  const [editingId, setEditingId] = React.useState<string | null>(null);
  const [editingTitle, setEditingTitle] = React.useState('');
  const [renamingId, setRenamingId] = React.useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = React.useState<ConversationSummary | null>(null);
  const [searchOpen, setSearchOpen] = React.useState(false);
  const viewportRef = React.useRef<HTMLDivElement>(null);
  const pendingPageRef = React.useRef<number | null>(null);
  const autoRequestedPageRef = React.useRef<number | null>(null);

  const startEditing = React.useCallback((conversation: ConversationSummary) => {
    setEditingId(conversation.id);
    setEditingTitle(conversation.title || '');
  }, []);

  const cancelEditing = React.useCallback(() => {
    setEditingId(null);
    setEditingTitle('');
  }, []);

  const submitEditing = React.useCallback(
    async (conversation: ConversationSummary) => {
      if (!onRename) return;
      const nextTitle = editingTitle.trim();
      if (!nextTitle || nextTitle === conversation.title) {
        cancelEditing();
        return;
      }

      setRenamingId(conversation.id);
      try {
        await onRename(conversation.id, nextTitle);
        cancelEditing();
      } finally {
        setRenamingId(current => (current === conversation.id ? null : current));
      }
    },
    [cancelEditing, editingTitle, onRename]
  );
  const requestDelete = React.useCallback((conversation: ConversationSummary) => {
    setDeleteTarget(conversation);
  }, []);

  const confirmDelete = React.useCallback(() => {
    if (!deleteTarget) return;
    onDelete(deleteTarget.id);
    setDeleteTarget(null);
  }, [deleteTarget, onDelete]);

  const backgroundStyle = getSidebarBackgroundStyle(backgroundImage);
  const conversationGroups = React.useMemo(
    () => groupByConversationTime(conversations, conversation => conversation.updatedAt),
    [conversations]
  );

  React.useEffect(() => {
    const pendingPage = pendingPageRef.current;
    if (pendingPage === null) return;
    if (!pagination?.hasMore || pagination.page >= pendingPage || !isLoadingList) {
      pendingPageRef.current = null;
    }
  }, [isLoadingList, pagination?.hasMore, pagination?.page]);

  const requestNextPage = React.useCallback(
    (source: 'scroll' | 'fill') => {
      if (!pagination?.hasMore || !onLoadMore || isLoadingList) return;

      const nextPage = pagination.page + 1;
      if (pendingPageRef.current === nextPage) return;
      if (source === 'fill' && autoRequestedPageRef.current === nextPage) return;
      if (source === 'fill') {
        autoRequestedPageRef.current = nextPage;
      }
      pendingPageRef.current = nextPage;
      void onLoadMore(nextPage);
    },
    [isLoadingList, onLoadMore, pagination?.hasMore, pagination?.page]
  );

  const handleHistoryScroll = React.useCallback(
    (event: React.UIEvent<HTMLDivElement>) => {
      const viewport = event.currentTarget;
      const nearBottom = viewport.scrollTop + viewport.clientHeight >= viewport.scrollHeight - 64;
      if (!nearBottom) return;

      requestNextPage('scroll');
    },
    [requestNextPage]
  );

  React.useEffect(() => {
    if (!pagination?.hasMore || isLoadingList) return;

    const frame = requestAnimationFrame(() => {
      const viewport = viewportRef.current;
      if (!viewport) return;
      const isViewportFilled = viewport.scrollHeight > viewport.clientHeight + 64;
      if (!isViewportFilled) {
        requestNextPage('fill');
      }
    });

    return () => cancelAnimationFrame(frame);
  }, [conversations.length, isLoadingList, pagination?.hasMore, requestNextPage]);

  return (
    <div
      className={cn(
        'border-r border-border flex flex-col shrink-0 transition-all duration-300 ease-in-out overflow-hidden h-full',
        isOpen ? 'w-64' : 'w-0 border-r-0',
        'bg-background',
        className
      )}
      style={backgroundStyle}
    >
      <div className="px-4 py-3 border-b border-border">
        <div
          className={cn(
            'flex w-full items-center justify-between gap-2',
            alwaysShowHeader ? 'mb-3' : 'mb-4',
            !alwaysShowHeader && 'sm:hidden'
          )}
        >
          {onClose && useBackNavigation ? (
            <Button
              variant="ghost"
              isIcon
              className={cn('h-6 w-6 text-muted-foreground', isOpen ? 'opacity-100' : 'opacity-0')}
              onClick={onClose}
              aria-label={t('common.back')}
            >
              <ArrowLeft className="h-4 w-4" />
            </Button>
          ) : null}
          <h2 className={cn('min-w-0 flex-1 truncate text-base font-semibold text-foreground')}>
            {t('webapp.chat.conversations')}
          </h2>
          {alwaysShowHeader ? (
            <Button
              variant="ghost"
              size="sm"
              className="h-8 shrink-0 gap-1.5 px-2.5 font-medium"
              onClick={onNewChat}
            >
              <MessageSquarePlus className="size-4" />
              {t('webapp.chat.newConversation')}
            </Button>
          ) : null}
          {onClose && !useBackNavigation ? (
            <Button
              variant="ghost"
              isIcon
              className={cn('h-6 w-6 text-muted-foreground', isOpen ? 'opacity-100' : 'opacity-0')}
              onClick={onClose}
              aria-label={t('common.close')}
            >
              <X className="h-4 w-4" />
            </Button>
          ) : null}
        </div>
        {!alwaysShowHeader ? (
          <Button
            className={cn(
              'w-full gap-2 font-bold overflow-hidden',
              isOpen ? 'opacity-100' : 'opacity-0'
            )}
            variant="default"
            onClick={onNewChat}
          >
            <MessageSquarePlus className="h-5 w-5" />
            {t('webapp.chat.newConversation')}
          </Button>
        ) : null}
        <Button
          className={cn(
            'w-full justify-start gap-2 overflow-hidden bg-muted/80 font-medium text-muted-foreground hover:bg-muted',
            !alwaysShowHeader && 'mt-2',
            isOpen ? 'opacity-100' : 'opacity-0'
          )}
          variant="ghost"
          onClick={() => setSearchOpen(true)}
        >
          <Search className="h-4 w-4" />
          {t('common.search')}
        </Button>
      </div>
      <ScrollArea
        className="h-0 grow"
        viewportRef={viewportRef}
        viewportProps={{ onScroll: handleHistoryScroll }}
      >
        <div className="p-2 space-y-1">
          {isLoadingList && conversations.length === 0 ? (
            <div className="flex h-40 items-center justify-center text-muted-foreground/70">
              <Loader2 className="size-5 animate-spin" />
            </div>
          ) : conversations.length === 0 ? (
            <div className="flex flex-col  items-center justify-center h-40 text-muted-foreground/60 text-sm px-4 text-center">
              <History className="h-8 w-8 mb-2 opacity-50" />
              <p className="line-clamp-1 overflow-hidden">{t('webapp.chat.noHistory')}</p>
              <p className="text-xs mt-1 opacity-70 line-clamp-1 overflow-hidden">
                {t('webapp.chat.startNewChat')}
              </p>
            </div>
          ) : (
            conversationGroups.map((group, groupIndex) => (
              <div key={group.key} className={cn('space-y-1', groupIndex > 0 && 'pt-2')}>
                <div className="px-2 pb-1 text-xs font-semibold text-muted-foreground">
                  {t(`webapp.chat.searchResultGroups.${group.key}`)}
                </div>
                {group.items.map(conv => {
                  const isRunning = isConversationRunning(conv);
                  const isEditing = editingId === conv.id;
                  const isRenaming = renamingId === conv.id;

                  return (
                    <ContextMenu key={conv.id}>
                      <ContextMenuTrigger asChild>
                        <div
                          onClick={() => {
                            if (!isEditing) {
                              onSelect(conv.id);
                            }
                          }}
                          onDoubleClick={event => {
                            if (!onRename || isEditing) return;
                            event.preventDefault();
                            event.stopPropagation();
                            startEditing(conv);
                          }}
                          className={cn(
                            'group flex items-center justify-between p-2 rounded-md text-sm cursor-pointer hover:bg-muted transition-colors',
                            activeId === conv.id && 'bg-muted font-medium'
                          )}
                        >
                          <div className="flex items-center w-full gap-2 overflow-hidden">
                            {isRunning ? (
                              <Loader2 className="h-4 w-4 shrink-0 animate-spin text-muted-foreground" />
                            ) : (
                              <History className="h-4 w-4 shrink-0 text-muted-foreground" />
                            )}
                            {isEditing ? (
                              <form
                                className="flex min-w-0 grow items-center gap-1"
                                onClick={event => event.stopPropagation()}
                                onSubmit={event => {
                                  event.preventDefault();
                                  void submitEditing(conv);
                                }}
                              >
                                <Input
                                  value={editingTitle}
                                  onChange={event => setEditingTitle(event.target.value)}
                                  onKeyDown={event => {
                                    if (event.key === 'Escape') {
                                      event.preventDefault();
                                      cancelEditing();
                                    }
                                  }}
                                  className="h-7 min-w-0 grow rounded-md px-2 text-xs"
                                  autoFocus
                                  disabled={isRenaming}
                                  aria-label={t('webapp.chat.renameConversation')}
                                />
                                <Button
                                  type="submit"
                                  variant="ghost"
                                  isIcon
                                  className="h-6 w-6 shrink-0"
                                  disabled={!editingTitle.trim() || isRenaming}
                                  aria-label={t('common.save')}
                                >
                                  {isRenaming ? (
                                    <Loader2 className="h-3 w-3 animate-spin" />
                                  ) : (
                                    <Check className="h-3 w-3" />
                                  )}
                                </Button>
                              </form>
                            ) : (
                              <div className="flex w-0 grow flex-col gap-0.5 overflow-hidden">
                                <span className="truncate">
                                  {conv.title || t('webapp.chat.newConversation')}
                                </span>
                              </div>
                            )}
                          </div>
                          <div
                            className="flex shrink-0 items-center"
                            onClick={e => e.stopPropagation()}
                          >
                            {onRename && !isEditing ? (
                              <Button
                                variant="ghost"
                                isIcon
                                className="h-6 w-6 sm:opacity-0 group-hover:opacity-100"
                                onClick={event => {
                                  event.stopPropagation();
                                  startEditing(conv);
                                }}
                                aria-label={t('webapp.chat.renameConversation')}
                              >
                                <Pencil className="h-3 w-3 text-muted-foreground hover:text-foreground" />
                              </Button>
                            ) : null}
                            {!isEditing ? (
                              <ConfirmDialog
                                variant="danger"
                                title={t('webapp.chat.deleteTitle')}
                                description={t('webapp.chat.deleteDescription', {
                                  conversationTitle: conv.title || t('webapp.chat.newConversation'),
                                })}
                                confirmText={t('common.delete')}
                                cancelText={t('common.cancel')}
                                onConfirm={() => onDelete(conv.id)}
                                trigger={
                                  <Button
                                    variant="ghost"
                                    isIcon
                                    className="h-6 w-6 sm:opacity-0 group-hover:opacity-100"
                                    onClick={e => e.stopPropagation()}
                                    aria-label={t('webapp.chat.deleteTitle')}
                                  >
                                    <Trash2 className="h-3 w-3 text-muted-foreground hover:text-destructive" />
                                  </Button>
                                }
                              />
                            ) : null}
                          </div>
                        </div>
                      </ContextMenuTrigger>
                      {!isEditing ? (
                        <ContextMenuContent alignOffset={4} className="w-40">
                          {onRename ? (
                            <ContextMenuItem onSelect={() => startEditing(conv)}>
                              <Pencil className="mr-2 h-4 w-4" />
                              {t('webapp.chat.renameConversation')}
                            </ContextMenuItem>
                          ) : null}
                          <ContextMenuItem
                            className="text-destructive focus:text-destructive"
                            onSelect={() => requestDelete(conv)}
                          >
                            <Trash2 className="mr-2 h-4 w-4" />
                            {t('webapp.chat.deleteConversation')}
                          </ContextMenuItem>
                        </ContextMenuContent>
                      ) : null}
                    </ContextMenu>
                  );
                })}
              </div>
            ))
          )}
          {pagination?.hasMore && isLoadingList ? (
            <div className="flex items-center justify-center gap-2 py-3 text-xs text-muted-foreground">
              <Loader2 className="size-3.5 animate-spin" />
              {t('webapp.chat.loadingConversations')}
            </div>
          ) : null}
          {!pagination?.hasMore && conversations.length > 0 && !isLoadingList ? (
            <div className="py-3 text-center text-xs text-muted-foreground/70">
              {t('webapp.chat.noMoreConversations')}
            </div>
          ) : null}
        </div>
      </ScrollArea>
      <ConfirmDialog
        variant="danger"
        open={deleteTarget !== null}
        onOpenChange={open => {
          if (!open) {
            setDeleteTarget(null);
          }
        }}
        title={t('webapp.chat.deleteTitle')}
        description={t('webapp.chat.deleteDescription', {
          conversationTitle: deleteTarget?.title || t('webapp.chat.newConversation'),
        })}
        confirmText={t('common.delete')}
        cancelText={t('common.cancel')}
        onConfirm={confirmDelete}
      />
      <ConversationSearchDialog
        open={searchOpen}
        conversations={conversations}
        activeId={activeId}
        onOpenChange={setSearchOpen}
        onSelect={id => {
          onSelect(id);
          onClose?.();
        }}
        onSelectSearchResult={
          onSelectSearchResult
            ? result => {
                onSelectSearchResult(result);
                onClose?.();
              }
            : undefined
        }
        search={search}
        searchKey={searchKey}
      />
    </div>
  );
}
