'use client';

import type { ReactNode, RefObject } from 'react';
import {
  ResourceSelectionCardIcon,
  ResourceSelectionDialog,
  ResourceSelectionEmptyState,
  ResourceSelectionGrid,
  ResourceSelectionPagination,
  ResourceSelectionSkeleton,
} from '@/components/common/resource-selection-dialog';
import { useT } from '@/i18n';

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
    <ResourceSelectionDialog
      open={open}
      title={title}
      description={description}
      selectedCountLabel={t('selectedCount', { count: selectedCount })}
      search={search}
      searchPlaceholder={searchPlaceholder}
      isSearching={isSearching}
      toolbar={toolbar}
      footer={footer}
      onOpenChange={onOpenChange}
      onChangeSearch={onChangeSearch}
    >
      {children}
    </ResourceSelectionDialog>
  );
}

export function AgentRuntimeSelectionGrid({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return <ResourceSelectionGrid className={className}>{children}</ResourceSelectionGrid>;
}

export function AgentRuntimeSelectionCardIcon({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return <ResourceSelectionCardIcon className={className}>{children}</ResourceSelectionCardIcon>;
}

export function AgentRuntimeSelectionSkeleton({ count = 8 }: { count?: number }) {
  return <ResourceSelectionSkeleton count={count} />;
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
    <ResourceSelectionEmptyState
      icon={icon}
      title={title}
      description={description}
      action={action}
      variant={variant}
      className={className}
    />
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
  return (
    <ResourceSelectionPagination
      sentinelRef={sentinelRef}
      isFetchingNextPage={isFetchingNextPage}
      hasNextPage={hasNextPage}
      hasItems={hasItems}
      loadingLabel={t('selectionDialog.loadingMore')}
      moreLabel={t('selectionDialog.scrollForMore')}
      endLabel={t('selectionDialog.noMore')}
    />
  );
}
