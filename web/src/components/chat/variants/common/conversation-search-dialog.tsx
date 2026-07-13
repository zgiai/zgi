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
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useT } from '@/i18n/translations';
import type {
  ConversationSearchFn,
  ConversationSearchResult,
  ConversationSummary,
} from '@/components/chat/controllers/types';
import { useQuery } from '@tanstack/react-query';
import {
  groupByConversationTime,
  type ConversationTimeGroup,
} from '@/components/chat/variants/common/conversation-time-groups';

const recentConversationLimit = 6;
const minRemoteSearchLength = 2;
const searchDebounceMs = 500;

interface ConversationSearchDialogProps {
  open: boolean;
  conversations: ConversationSummary[];
  activeId: string | null;
  onOpenChange: (open: boolean) => void;
  onSelect: (id: string) => void;
  onSelectSearchResult?: (result: ConversationSearchResult) => void;
  search?: ConversationSearchFn;
  searchKey?: readonly unknown[];
}

interface ConversationSearchItemProps {
  title: React.ReactNode;
  titleText: string;
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

function isHttpNotFoundError(error: unknown): boolean {
  if (!error || typeof error !== 'object') return false;

  const directStatus = (error as { status?: unknown }).status;
  if (directStatus === 404) return true;

  const responseStatus = (error as { response?: { status?: unknown } }).response?.status;
  return responseStatus === 404;
}

function searchLocalConversations(
  conversations: ConversationSummary[],
  normalizedQuery: string,
  fallbackTitle: string,
  limit: number
): ConversationSearchResult[] {
  if (!normalizedQuery) return [];

  return conversations
    .filter(conversation =>
      getConversationTitle(conversation, fallbackTitle).toLowerCase().includes(normalizedQuery)
    )
    .slice(0, limit)
    .map(conversation => {
      const title = getConversationTitle(conversation, fallbackTitle);

      return {
        type: 'conversation',
        conversationId: conversation.id,
        conversationTitle: title,
        snippet: title,
        updatedAt: conversation.updatedAt,
      };
    });
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

function renderHighlightedText(text: string, query: string): React.ReactNode {
  if (!query) return text;

  const normalizedText = text.toLowerCase();
  const normalizedQuery = query.toLowerCase();
  const parts: React.ReactNode[] = [];
  let searchStart = 0;
  let key = 0;

  while (searchStart < text.length) {
    const matchIndex = normalizedText.indexOf(normalizedQuery, searchStart);
    if (matchIndex === -1) {
      parts.push(text.slice(searchStart));
      break;
    }

    if (matchIndex > searchStart) {
      parts.push(text.slice(searchStart, matchIndex));
    }

    const matchEnd = matchIndex + query.length;
    parts.push(
      <mark
        key={`match-${key}`}
        className="rounded-sm bg-[rgb(235,235,235)] px-0.5 font-semibold text-foreground"
      >
        {text.slice(matchIndex, matchEnd)}
      </mark>
    );
    searchStart = matchEnd;
    key += 1;
  }

  return parts;
}

function ConversationSearchItem({
  title,
  titleText,
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
        <span className="block truncate font-medium text-foreground" title={titleText}>
          {title}
        </span>
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
  onSelectSearchResult,
  search,
  searchKey,
}: ConversationSearchDialogProps) {
  const t = useT();
  const inputRef = React.useRef<HTMLInputElement>(null);
  const [query, setQuery] = React.useState('');
  const normalizedQuery = normalizeSearchText(query);
  const debouncedQuery = useDebouncedValue(normalizedQuery, searchDebounceMs);
  const hasRemoteSearch = typeof search === 'function';
  const canSearchRemote = hasRemoteSearch && normalizedQuery.length >= minRemoteSearchLength;
  const debouncedCanSearchRemote =
    debouncedQuery.length >= minRemoteSearchLength && debouncedQuery === normalizedQuery;
  const searchEnabled = open && canSearchRemote && debouncedCanSearchRemote;
  const newConversationText = t('webapp.chat.newConversation');
  const searchQuery = useQuery<ConversationSearchResult[]>({
    queryKey: [...(searchKey ?? ['conversation-search']), debouncedQuery, 20],
    queryFn: async () => {
      if (!search) return [];

      try {
        return await search(debouncedQuery, 20);
      } catch (error) {
        if (isHttpNotFoundError(error)) {
          return searchLocalConversations(conversations, debouncedQuery, newConversationText, 20);
        }

        throw error;
      }
    },
    enabled: searchEnabled,
    retry: false,
    staleTime: 60_000,
    refetchOnMount: false,
    refetchOnReconnect: false,
    refetchOnWindowFocus: false,
  });

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
  const groupedVisibleConversations = React.useMemo(
    () => groupByConversationTime(visibleConversations, conversation => conversation.updatedAt),
    [visibleConversations]
  );
  const visibleConversationGroups = React.useMemo<
    Array<ConversationTimeGroup<ConversationSummary>>
  >(() => groupedVisibleConversations, [groupedVisibleConversations]);

  const handleSelect = React.useCallback(
    (id: string) => {
      onSelect(id);
      onOpenChange(false);
    },
    [onOpenChange, onSelect]
  );
  const handleSearchResultSelect = React.useCallback(
    (result: ConversationSearchResult) => {
      if (onSelectSearchResult) {
        onSelectSearchResult(result);
      } else {
        onSelect(result.conversationId);
      }
      onOpenChange(false);
    },
    [onOpenChange, onSelect, onSelectSearchResult]
  );
  const searchResults = React.useMemo(
    () => (searchQuery.data ?? []) as ConversationSearchResult[],
    [searchQuery.data]
  );
  const groupedSearchResults = React.useMemo(
    () => groupByConversationTime(searchResults, result => result.updatedAt),
    [searchResults]
  );
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
          {hasQuery ? (
            <div className="shrink-0 bg-background px-1 py-2">
              <div className="text-sm font-semibold text-foreground">{t('common.results')}</div>
            </div>
          ) : null}
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
                  groupedSearchResults.map((group, groupIndex) => (
                    <div
                      key={group.key}
                      className={cn('space-y-1', groupIndex > 0 && 'border-t pt-2')}
                    >
                      <div className="px-3 pb-1 text-xs font-semibold text-muted-foreground">
                        {t(`webapp.chat.searchResultGroups.${group.key}`)}
                      </div>
                      {group.items.map(result => {
                        const title = result.conversationTitle?.trim() || newConversationText;
                        const updatedAt = formatConversationTime(result.updatedAt);
                        const isActive = result.conversationId === activeId;

                        return (
                          <ConversationSearchItem
                            key={`${result.type}-${result.conversationId}-${result.messageId ?? 'title'}`}
                            title={renderHighlightedText(title, normalizedQuery)}
                            titleText={title}
                            meta={
                              <>
                                {renderHighlightedText(result.snippet, normalizedQuery)}
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
                      })}
                    </div>
                  ))
                )
              ) : visibleConversations.length === 0 ? (
                <div className="flex min-h-40 flex-col items-center justify-center px-6 text-center text-sm text-muted-foreground">
                  <Search className="mb-3 size-8 opacity-50" />
                  <div className="font-medium text-foreground">{t('webapp.chat.noHistory')}</div>
                  <div className="mt-1 text-xs">{t('webapp.chat.startNewChat')}</div>
                </div>
              ) : (
                visibleConversationGroups.map((group, groupIndex) => (
                  <div
                    key={group.key}
                    className={cn('space-y-1', groupIndex > 0 && 'border-t pt-2')}
                  >
                    <div className="px-3 pb-1 text-xs font-semibold text-muted-foreground">
                      {t(`webapp.chat.searchResultGroups.${group.key}`)}
                    </div>
                    {group.items.map(conversation => {
                      const title = getConversationTitle(conversation, newConversationText);
                      const updatedAt = formatConversationTime(conversation.updatedAt);
                      const isActive = conversation.id === activeId;

                      return (
                        <ConversationSearchItem
                          key={conversation.id}
                          title={renderHighlightedText(title, normalizedQuery)}
                          titleText={title}
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
                    })}
                  </div>
                ))
              )}
            </div>
          </ScrollArea>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
