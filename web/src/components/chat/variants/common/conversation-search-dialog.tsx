'use client';

import * as React from 'react';
import { History, Loader2, MessageSquare, Search, X } from 'lucide-react';
import {
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { ScrollArea } from '@/components/ui/scroll-area';
import { cn } from '@/lib/utils';
import { useAIChatSearch } from '@/hooks/aichat/use-aichat-search';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useT } from '@/i18n/translations';
import type { ConversationSummary } from '@/components/chat/controllers/types';
import type { AIChatSearchResult } from '@/services/types/aichat';

const recentConversationLimit = 6;
const minRemoteSearchLength = 2;
const searchDebounceMs = 500;

interface ConversationSearchDialogProps {
  open: boolean;
  conversations: ConversationSummary[];
  activeId: string | null;
  onOpenChange: (open: boolean) => void;
  onSelect: (id: string) => void;
}

interface ConversationSearchItemProps {
  title: string;
  meta: React.ReactNode;
  icon: React.ReactNode;
  isActive: boolean;
  onClick: () => void;
}

function normalizeSearchText(value: string): string {
  return value.trim().toLowerCase();
}

function getConversationTitle(conversation: ConversationSummary, fallback: string): string {
  return conversation.title?.trim() || fallback;
}

function formatConversationTime(updatedAt?: number): string {
  if (!updatedAt) return '';
  const timestamp = updatedAt < 1_000_000_000_000 ? updatedAt * 1000 : updatedAt;
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(timestamp));
}

function ConversationSearchItem({
  title,
  meta,
  icon,
  isActive,
  onClick,
}: ConversationSearchItemProps) {
  return (
    <button
      type="button"
      className={cn(
        'flex w-full min-w-0 items-center gap-3 overflow-hidden rounded-lg px-3 py-2.5 text-left text-sm transition-colors hover:bg-muted',
        isActive && 'bg-muted'
      )}
      onClick={onClick}
    >
      <span className="flex size-8 shrink-0 items-center justify-center rounded-md text-muted-foreground">
        {icon}
      </span>
      <span className="w-0 grow overflow-hidden">
        <span className="block truncate font-medium text-foreground">{title}</span>
        <span className="mt-0.5 block truncate text-xs text-muted-foreground">{meta}</span>
      </span>
    </button>
  );
}

export function ConversationSearchDialog({
  open,
  conversations,
  activeId,
  onOpenChange,
  onSelect,
}: ConversationSearchDialogProps) {
  const t = useT();
  const inputRef = React.useRef<HTMLInputElement>(null);
  const [query, setQuery] = React.useState('');
  const normalizedQuery = normalizeSearchText(query);
  const debouncedQuery = useDebouncedValue(normalizedQuery, searchDebounceMs);
  const canSearchRemote = normalizedQuery.length >= minRemoteSearchLength;
  const debouncedCanSearchRemote =
    debouncedQuery.length >= minRemoteSearchLength && debouncedQuery === normalizedQuery;
  const searchEnabled = open && canSearchRemote && debouncedCanSearchRemote;
  const searchQuery = useAIChatSearch(debouncedQuery, 20, { enabled: searchEnabled });
  const newConversationText = t('webapp.chat.newConversation');

  React.useEffect(() => {
    if (!open) {
      setQuery('');
      return;
    }
    requestAnimationFrame(() => {
      inputRef.current?.focus();
    });
  }, [open]);

  const visibleConversations = React.useMemo(() => {
    if (!normalizedQuery) return conversations.slice(0, recentConversationLimit);
    return conversations.filter(conversation =>
      getConversationTitle(conversation, newConversationText)
        .toLowerCase()
        .includes(normalizedQuery)
    );
  }, [conversations, newConversationText, normalizedQuery]);

  const handleSelect = React.useCallback(
    (id: string) => {
      onSelect(id);
      onOpenChange(false);
    },
    [onOpenChange, onSelect]
  );
  const handleSearchResultSelect = React.useCallback(
    (result: AIChatSearchResult) => {
      onSelect(result.conversation_id);
      onOpenChange(false);
    },
    [onOpenChange, onSelect]
  );

  const searchResults = searchQuery.data ?? [];
  const hasQuery = normalizedQuery.length > 0;
  const showRemoteSearchResults = canSearchRemote;
  const isWaitingForDebounce = canSearchRemote && normalizedQuery !== debouncedQuery;
  const isSearching = showRemoteSearchResults && (isWaitingForDebounce || searchQuery.isFetching);
  const searchFailed = showRemoteSearchResults && searchQuery.isError;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        size="lg"
        showCloseButton={false}
        className="flex h-[min(570px,calc(100vh-2rem))] flex-col overflow-hidden p-7 sm:max-w-2xl"
      >
        <DialogClose className="absolute right-4 top-3.5 z-10 rounded-full p-1 text-muted-foreground/70 transition-colors hover:bg-muted hover:text-foreground focus:outline-none focus:ring-1 focus:ring-ring">
          <X className="size-4" />
          <span className="sr-only">Close</span>
        </DialogClose>
        <DialogHeader className="shrink-0 p-0 pt-3">
          <DialogTitle className="sr-only">{t('common.search')}</DialogTitle>
          <div className="relative">
            <Input
              ref={inputRef}
              type="text"
              leftIcon={<Search />}
              value={query}
              onChange={event => setQuery(event.target.value)}
              placeholder={t('common.form.searchPlaceholder')}
              className="h-11 rounded-xl border-border/80 bg-background pr-10 text-base shadow-sm transition-all hover:border-border-strong focus-visible:border-primary focus-visible:shadow-sm"
              aria-label={t('common.search')}
            />
            {query ? (
              <button
                type="button"
                className="absolute right-2 top-1/2 z-10 flex size-7 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground/70 transition-colors hover:bg-muted hover:text-foreground focus:outline-none focus:ring-1 focus:ring-ring"
                onClick={() => {
                  setQuery('');
                  inputRef.current?.focus();
                }}
                aria-label={t('common.clear')}
              >
                <X className="size-3.5" />
              </button>
            ) : null}
          </div>
        </DialogHeader>
        <DialogBody className="mt-4 flex min-h-0 flex-1 flex-col overflow-hidden p-0">
          <div className="shrink-0 bg-background px-1 py-2 text-sm font-semibold text-foreground">
            {hasQuery ? t('common.results') : t('webapp.chat.recentConversations')}
          </div>
          <ScrollArea className="min-h-0 flex-1" viewportProps={{ className: 'pr-6' }}>
            <div className="space-y-1 px-1 py-2">
              {showRemoteSearchResults ? (
                isSearching ? (
                  <div className="flex min-h-40 flex-col items-center justify-center px-6 text-center text-sm text-muted-foreground">
                    <Loader2 className="mb-3 size-8 animate-spin opacity-60" />
                    <div className="font-medium text-foreground">{t('common.search')}</div>
                  </div>
                ) : searchFailed || searchResults.length === 0 ? (
                  <div className="flex min-h-40 flex-col items-center justify-center px-6 text-center text-sm text-muted-foreground">
                    <Search className="mb-3 size-8 opacity-50" />
                    <div className="font-medium text-foreground">{t('webapp.chat.noHistory')}</div>
                    <div className="mt-1 text-xs">{t('webapp.chat.startNewChat')}</div>
                  </div>
                ) : (
                  searchResults.map(result => {
                    const title = result.conversation_title?.trim() || newConversationText;
                    const updatedAt = formatConversationTime(result.updated_at);
                    const isActive = result.conversation_id === activeId;

                    return (
                      <ConversationSearchItem
                        key={`${result.type}-${result.conversation_id}-${result.message_id ?? 'title'}`}
                        title={title}
                        meta={
                          <>
                            {result.snippet}
                            {updatedAt ? ` - ${updatedAt}` : ''}
                          </>
                        }
                        icon={
                          result.type === 'message' ? (
                            <MessageSquare className="size-4" />
                          ) : (
                            <History className="size-4" />
                          )
                        }
                        isActive={isActive}
                        onClick={() => handleSearchResultSelect(result)}
                      />
                    );
                  })
                )
              ) : visibleConversations.length === 0 ? (
                <div className="flex min-h-40 flex-col items-center justify-center px-6 text-center text-sm text-muted-foreground">
                  <Search className="mb-3 size-8 opacity-50" />
                  <div className="font-medium text-foreground">{t('webapp.chat.noHistory')}</div>
                  <div className="mt-1 text-xs">{t('webapp.chat.startNewChat')}</div>
                </div>
              ) : (
                visibleConversations.map(conversation => {
                  const title = getConversationTitle(conversation, newConversationText);
                  const updatedAt = formatConversationTime(conversation.updatedAt);
                  const isActive = conversation.id === activeId;

                  return (
                    <ConversationSearchItem
                      key={conversation.id}
                      title={title}
                      meta={
                        <span className="inline-flex min-w-0 max-w-full items-center gap-1.5 overflow-hidden">
                          <MessageSquare className="size-3 shrink-0" />
                          <span className="shrink-0">
                            {t('webapp.chat.messageCount', {
                              count: conversation.dialogueCount ?? 0,
                            })}
                          </span>
                          {updatedAt ? <span className="truncate">- {updatedAt}</span> : null}
                        </span>
                      }
                      icon={<History className="size-4" />}
                      isActive={isActive}
                      onClick={() => handleSelect(conversation.id)}
                    />
                  );
                })
              )}
            </div>
          </ScrollArea>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
