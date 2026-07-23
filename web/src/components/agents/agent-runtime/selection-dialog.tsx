'use client';

import type { ReactNode, RefObject } from 'react';
import { Loader2, Search } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

interface AgentRuntimeSelectionDialogProps {
  open: boolean;
  title: string;
  description: string;
  selectedCount: number;
  search: string;
  searchPlaceholder: string;
  isSearching?: boolean;
  toolbar?: ReactNode;
  children: ReactNode;
  footer: ReactNode;
  onOpenChange: (open: boolean) => void;
  onChangeSearch: (value: string) => void;
}

export function AgentRuntimeSelectionDialog({
  open,
  title,
  description,
  selectedCount,
  search,
  searchPlaceholder,
  isSearching = false,
  toolbar,
  children,
  footer,
  onOpenChange,
  onChangeSearch,
}: AgentRuntimeSelectionDialogProps) {
  const t = useT('agents.agentRuntime');

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        size="full"
        className="h-[min(780px,calc(100vh-2rem))] max-w-[min(1440px,calc(100vw-2rem))]"
      >
        <DialogHeader className="shrink-0 border-b">
          <div className="flex items-start justify-between gap-4 pr-8">
            <div className="min-w-0">
              <DialogTitle>{title}</DialogTitle>
              <DialogDescription>{description}</DialogDescription>
            </div>
            <Badge variant="subtle" className="h-8 shrink-0 rounded-md px-3 font-normal">
              {t('selectedCount', { count: selectedCount })}
            </Badge>
          </div>
        </DialogHeader>
        <DialogBody className="flex min-h-0 flex-col overflow-hidden p-0">
          <div className="flex shrink-0 flex-col gap-3 border-b bg-muted/20 px-6 py-3 sm:flex-row sm:items-center">
            <div className="relative min-w-0 flex-1 sm:max-w-md">
              <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={search}
                onChange={event => onChangeSearch(event.target.value)}
                placeholder={searchPlaceholder}
                className="h-9 bg-background pl-9 pr-9"
              />
              {isSearching ? (
                <Loader2 className="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 animate-spin text-muted-foreground" />
              ) : null}
            </div>
            {toolbar}
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto px-6 py-4">{children}</div>
        </DialogBody>
        <DialogFooter className="shrink-0 border-t">{footer}</DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function AgentRuntimeSelectionGrid({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        'grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4',
        className
      )}
    >
      {children}
    </div>
  );
}

export function AgentRuntimeSelectionCardIcon({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <span
      className={cn(
        'flex size-8 shrink-0 items-center justify-center rounded-md border bg-background text-muted-foreground [&>svg]:size-4',
        className
      )}
    >
      {children}
    </span>
  );
}

export function AgentRuntimeSelectionSkeleton({ count = 8 }: { count?: number }) {
  return (
    <AgentRuntimeSelectionGrid>
      {Array.from({ length: count }).map((_, index) => (
        <Skeleton key={index} className="h-36 w-full rounded-lg" />
      ))}
    </AgentRuntimeSelectionGrid>
  );
}

export function AgentRuntimeSelectionEmptyState({
  icon,
  title,
  description,
  action,
  variant = 'resource',
  className,
}: {
  icon: ReactNode;
  title: string;
  description: string;
  action?: ReactNode;
  variant?: 'resource' | 'search';
  className?: string;
}) {
  return (
    <div
      className={cn(
        'relative flex h-full min-h-64 flex-col items-center justify-center overflow-hidden rounded-xl border border-dashed bg-gradient-to-b from-muted/30 via-background to-background px-6 py-10 text-center',
        className
      )}
    >
      <div
        className={cn(
          'mb-4 flex size-12 items-center justify-center rounded-full border bg-background shadow-sm [&>svg]:size-5',
          variant === 'search' ? 'text-muted-foreground' : 'border-primary/20 text-primary'
        )}
      >
        {icon}
      </div>
      <h3 className="text-base font-semibold text-foreground">{title}</h3>
      <p className="mt-2 max-w-md text-sm leading-6 text-muted-foreground">{description}</p>
      {action ? <div className="mt-5">{action}</div> : null}
    </div>
  );
}

export function AgentRuntimeSelectionPagination({
  sentinelRef,
  isFetchingNextPage,
  hasNextPage,
  hasItems,
}: {
  sentinelRef: RefObject<HTMLDivElement>;
  isFetchingNextPage: boolean;
  hasNextPage: boolean;
  hasItems: boolean;
}) {
  const t = useT('agents.agentRuntime');

  if (!hasItems) return null;

  return (
    <div
      ref={sentinelRef}
      role="status"
      className="flex min-h-12 items-center justify-center pt-4 text-xs text-muted-foreground"
    >
      {isFetchingNextPage ? (
        <span className="flex items-center gap-2">
          <Loader2 className="size-3.5 animate-spin" />
          {t('selectionDialog.loadingMore')}
        </span>
      ) : hasNextPage ? (
        t('selectionDialog.scrollForMore')
      ) : (
        t('selectionDialog.noMore')
      )}
    </div>
  );
}
