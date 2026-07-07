'use client';

import { useEffect, useMemo, useState } from 'react';
import { useT } from '@/i18n';
import { useRouter } from 'next/navigation';
import {
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerClose,
} from '@/components/ui/drawer';
import { Input } from '@/components/ui/input';
import { Search, X, User, Info, Loader2, RefreshCw } from 'lucide-react';
import { useRoleMembers } from '@/hooks/organization/use-role-members';
import { useRoleActions } from '@/hooks/organization/use-role-actions';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Checkbox } from '@/components/ui/checkbox';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { cn } from '@/lib/utils';
import { useInfiniteObserver } from '@/hooks/use-infinite-observer';
import type { ApplyRoleTemplateTarget } from '@/services/types/organization';

interface RoleMembersDrawerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  roleId: string | null;
  roleName: string;
  canApplyTemplate?: boolean;
}

export function RoleMembersDrawer({
  open,
  onOpenChange,
  roleId,
  roleName,
  canApplyTemplate = true,
}: RoleMembersDrawerProps) {
  const t = useT('dashboard');
  const router = useRouter();
  const [selectedTargetKeys, setSelectedTargetKeys] = useState<Set<string>>(new Set());
  const [confirmOpen, setConfirmOpen] = useState(false);
  const { applyRoleTemplate, isApplyingTemplate } = useRoleActions();
  const {
    members,
    total,
    isLoading,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
    searchKeyword,
    setSearchKeyword,
    refetch,
  } = useRoleMembers(roleId, open && !!roleId);

  useEffect(() => {
    if (!open) {
      setSelectedTargetKeys(new Set());
      setConfirmOpen(false);
    }
  }, [open, roleId]);

  const targetByKey = useMemo(() => {
    const map = new Map<string, ApplyRoleTemplateTarget>();
    members.forEach(member => {
      (member.workspaces ?? []).forEach(workspace => {
        const key = `${member.account_id}:${workspace.workspace_id}`;
        map.set(key, {
          account_id: member.account_id,
          workspace_id: workspace.workspace_id,
        });
      });
    });
    return map;
  }, [members]);

  const loadedTargetKeys = useMemo(() => Array.from(targetByKey.keys()), [targetByKey]);
  const selectedTargets = useMemo(
    () =>
      Array.from(selectedTargetKeys)
        .map(key => targetByKey.get(key))
        .filter((target): target is ApplyRoleTemplateTarget => Boolean(target)),
    [selectedTargetKeys, targetByKey]
  );
  const selectedLoadedCount = loadedTargetKeys.filter(key => selectedTargetKeys.has(key)).length;
  const allLoadedSelected =
    loadedTargetKeys.length > 0 && selectedLoadedCount === loadedTargetKeys.length;
  const partiallySelected =
    selectedLoadedCount > 0 && selectedLoadedCount < loadedTargetKeys.length;

  const toggleTarget = (key: string, checked: boolean) => {
    setSelectedTargetKeys(prev => {
      const next = new Set(prev);
      if (checked) {
        next.add(key);
      } else {
        next.delete(key);
      }
      return next;
    });
  };

  const toggleLoadedTargets = (checked: boolean) => {
    setSelectedTargetKeys(prev => {
      const next = new Set(prev);
      loadedTargetKeys.forEach(key => {
        if (checked) {
          next.add(key);
        } else {
          next.delete(key);
        }
      });
      return next;
    });
  };

  const handleApplyTemplate = async () => {
    if (!canApplyTemplate || !roleId || selectedTargets.length === 0) return;
    await applyRoleTemplate({
      roleId,
      data: {
        members: selectedTargets,
      },
    });
    setSelectedTargetKeys(new Set());
    await refetch();
  };

  // Infinite scroll sentinel
  const sentinelRef = useInfiniteObserver({
    hasNextPage,
    isFetchingNextPage,
    fetchNextPage,
    rootMargin: '100px',
  });

  return (
    <Drawer open={open} onOpenChange={onOpenChange} direction="right">
      <DrawerContent className="h-full w-full sm:max-w-lg flex flex-col">
        <DrawerHeader className="border-b flex-shrink-0">
          <div className="flex items-center justify-between">
            <DrawerTitle className="text-lg font-bold">
              {roleName} - {t('organization.permissions.associatedMembers')}
            </DrawerTitle>
            <DrawerClose asChild>
              <Button variant="ghost" isIcon className="h-8 w-8">
                <X className="h-4 w-4" />
              </Button>
            </DrawerClose>
          </div>
        </DrawerHeader>

        <div className="flex flex-col flex-1 min-h-0">
          {/* Search Bar */}
          <div className="p-4 border-b">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground pointer-events-none" />
              <Input
                placeholder={t('organization.permissions.searchMembersPlaceholder')}
                value={searchKeyword}
                onChange={e => setSearchKeyword(e.target.value)}
                maxLength={100}
                className="pl-9"
              />
            </div>
            {canApplyTemplate && loadedTargetKeys.length > 0 && (
              <div className="mt-3 flex items-center justify-between gap-3 rounded-md border bg-muted/30 px-3 py-2">
                <label className="flex min-w-0 items-center gap-2 text-sm text-muted-foreground">
                  <Checkbox
                    checked={
                      allLoadedSelected ? true : partiallySelected ? 'indeterminate' : false
                    }
                    onCheckedChange={checked => toggleLoadedTargets(checked === true)}
                  />
                  <span className="truncate">
                    {t('organization.permissions.selectedTargets', {
                      count: selectedTargets.length,
                    })}
                  </span>
                </label>
                <Button
                  size="sm"
                  className="shrink-0"
                  disabled={selectedTargets.length === 0 || isApplyingTemplate}
                  onClick={() => setConfirmOpen(true)}
                >
                  {isApplyingTemplate ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <RefreshCw className="h-4 w-4" />
                  )}
                  <span>
                    {isApplyingTemplate
                      ? t('organization.permissions.applyingTemplate')
                      : t('organization.permissions.applyCurrentTemplate')}
                  </span>
                </Button>
              </div>
            )}
          </div>

          {/* Members List */}
          <div className="flex-1 overflow-y-auto p-4">
            {isLoading ? (
              <div className="space-y-4">
                {Array.from({ length: 3 }).map((_, index) => (
                  <div key={`skeleton-${index}`} className="space-y-2">
                    <Skeleton className="h-6 w-32" />
                    <Skeleton className="h-4 w-48" />
                    <Skeleton className="h-3 w-full" />
                  </div>
                ))}
              </div>
            ) : members.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-full text-center text-muted-foreground">
                <User className="h-12 w-12 mb-4 opacity-50" />
                <p className="text-sm">{t('organization.permissions.noMembersFound')}</p>
              </div>
            ) : (
              <>
                <div className="space-y-4">
                  {members.map((member, index) => (
                    <div
                      key={member.account_id}
                      className={cn(
                        'flex items-start gap-3 pb-4',
                        index !== members.length - 1 && 'border-b'
                      )}
                    >
                      <div className="flex-1 min-w-0">
                        <h4 className="font-semibold text-base mb-1 truncate">
                          {member.member_name || member.name}
                        </h4>
                        <div className="text-sm text-muted-foreground">{member.email}</div>
                        {(member.workspaces ?? []).length > 0 ? (
                          <div className="mt-3 space-y-2">
                            {(member.workspaces ?? []).map(workspace => {
                              const key = `${member.account_id}:${workspace.workspace_id}`;
                              const selected = selectedTargetKeys.has(key);
                              return (
                                <label
                                  key={key}
                                  className={cn(
                                    'flex items-center gap-3 rounded-md border px-3 py-2 transition-colors',
                                    canApplyTemplate ? 'cursor-pointer' : 'cursor-default',
                                    selected
                                      ? 'border-brand-main/50 bg-brand-subtle'
                                      : 'hover:bg-muted/50'
                                  )}
                                >
                                  {canApplyTemplate ? (
                                    <Checkbox
                                      checked={selected}
                                      onCheckedChange={checked =>
                                        toggleTarget(key, checked === true)
                                      }
                                    />
                                  ) : null}
                                  <div className="min-w-0 flex-1">
                                    <div className="truncate text-sm font-medium">
                                      {workspace.workspace_name || workspace.workspace_id}
                                    </div>
                                    <div className="mt-1 flex min-w-0 items-center gap-2">
                                      <Badge variant="subtle">
                                        {workspace.role_name || workspace.role}
                                      </Badge>
                                    </div>
                                  </div>
                                </label>
                              );
                            })}
                          </div>
                        ) : (
                          <p className="mt-3 text-xs text-muted-foreground">
                            {t('organization.permissions.noAppliedWorkspaces')}
                          </p>
                        )}
                      </div>
                      <div className="flex-shrink-0">
                        <User className="h-5 w-5 text-muted-foreground" />
                      </div>
                    </div>
                  ))}
                </div>

                {/* Infinite scroll sentinel */}
                <div ref={sentinelRef} className="py-4 text-center">
                  {isFetchingNextPage && (
                    <div className="flex items-center justify-center gap-2 text-muted-foreground">
                      <Loader2 className="h-4 w-4 animate-spin" />
                      <span className="text-sm">{t('organization.permissions.loadingMore')}</span>
                    </div>
                  )}
                  {!hasNextPage && members.length > 0 && (
                    <p className="text-xs text-muted-foreground">
                      {t('organization.permissions.allMembersLoaded', { total })}
                    </p>
                  )}
                </div>
              </>
            )}
          </div>

          {/* Info Footer */}
          <div className="p-4 border-t bg-brand-subtle flex-shrink-0">
            <div className="flex items-start gap-2">
              <Info className="h-4 w-4 text-brand-main mt-0.5 flex-shrink-0" />
              <p className="text-xs text-brand-strong">
                {canApplyTemplate
                  ? t('organization.permissions.membersListInfo')
                  : t('organization.permissions.fixedMembersListInfo')}
                <span
                  className="text-brand-main cursor-pointer hover:underline"
                  onClick={() => {
                    router.push('/dashboard/organization/workspaces');
                    onOpenChange(false);
                  }}
                >
                  {t('organization.permissions.workspaceManagement')}
                </span>
                {t('organization.permissions.or')}
                <span
                  className="text-brand-main cursor-pointer hover:underline"
                  onClick={() => {
                    router.push('/dashboard/organization/contacts');
                    onOpenChange(false);
                  }}
                >
                  {t('organization.permissions.addressBook')}
                </span>
                {t('organization.permissions.toOperate')}
              </p>
            </div>
          </div>
        </div>
        <ConfirmDialog
          open={confirmOpen}
          onOpenChange={setConfirmOpen}
          title={t('organization.permissions.applyTemplateConfirmTitle')}
          description={t('organization.permissions.applyTemplateConfirmDescription', {
            count: selectedTargets.length,
          })}
          confirmText={t('organization.permissions.applyTemplateConfirm')}
          cancelText={t('organization.permissions.applyTemplateCancel')}
          loading={isApplyingTemplate}
          onConfirm={() => {
            void handleApplyTemplate();
          }}
        />
      </DrawerContent>
    </Drawer>
  );
}
