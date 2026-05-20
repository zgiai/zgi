'use client';

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
import { Search, X, User, Info, Loader2 } from 'lucide-react';
import { useRoleMembers } from '@/hooks/organization/use-role-members';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { useInfiniteObserver } from '@/hooks/use-infinite-observer';

interface RoleMembersDrawerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  roleId: string | null;
  roleName: string;
}

export function RoleMembersDrawer({
  open,
  onOpenChange,
  roleId,
  roleName,
}: RoleMembersDrawerProps) {
  const t = useT('dashboard');
  const router = useRouter();
  const {
    members,
    total,
    isLoading,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
    searchKeyword,
    setSearchKeyword,
  } = useRoleMembers(roleId, open && !!roleId);

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
                      </div>
                      <div className="flex-shrink-0">
                        <User className="h-5 w-5 text-muted-foreground" />
                      </div>
                    </div>
                  ))}
                </div>

                {/* Infinite scroll sentinel */}
                {!searchKeyword && (
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
                )}
              </>
            )}
          </div>

          {/* Info Footer */}
          <div className="p-4 border-t bg-brand-subtle flex-shrink-0">
            <div className="flex items-start gap-2">
              <Info className="h-4 w-4 text-brand-main mt-0.5 flex-shrink-0" />
              <p className="text-xs text-brand-strong">
                {t('organization.permissions.membersListInfo')}
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
      </DrawerContent>
    </Drawer>
  );
}
