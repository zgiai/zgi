'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Loader2, Search, Users } from 'lucide-react';
import { toast } from 'sonner';

import { Badge } from '@/components/ui/badge';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
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
import { Pagination } from '@/components/ui/pagination';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useOrganizationRoles } from '@/hooks/organization/use-organization-roles';
import { useWorkspaceMemberActions } from '@/hooks/workspace/use-workspace-member-actions';
import { useWorkspaces } from '@/hooks/workspace/use-workspaces';
import { useLocale } from '@/hooks/use-locale';
import type { DepartmentMember } from '@/services/types/organization';
import type { WorkspaceManagement } from '@/services/types/workspace';
import { cn } from '@/lib/utils';
import { pickLocale } from '@/utils/tool-helpers';
import {
  isAssignableWorkspaceAdminRole,
  isSelectableWorkspacePermissionTemplate,
} from '@/utils/workspace-role-templates';

interface AssignWorkspaceDialogProps {
  open: boolean;
  member: DepartmentMember | null;
  onOpenChange: (open: boolean) => void;
  onAssigned?: () => void;
}

export function AssignWorkspaceDialog({
  open,
  member,
  onOpenChange,
  onAssigned,
}: AssignWorkspaceDialogProps) {
  const t = useT('dashboard.organization.contacts.assignWorkspaceDialog');
  const [searchKeyword, setSearchKeyword] = useState('');
  const [selectedWorkspaceId, setSelectedWorkspaceId] = useState('');
  const [selectedRoleId, setSelectedRoleId] = useState('');
  const [shouldSetAdmin, setShouldSetAdmin] = useState(false);
  const [currentPage, setCurrentPage] = useState(1);
  const debouncedSearchKeyword = useDebouncedValue(searchKeyword, 400);
  const pageSize = 8;

  const {
    workspaces,
    total,
    hasMore,
    isLoading: isLoadingWorkspaces,
    isPlaceholderData,
  } = useWorkspaces(debouncedSearchKeyword, currentPage, pageSize, {
    keepPreviousData: true,
    enabled: open,
  });
  const { roles, isLoading: isLoadingRoles } = useOrganizationRoles({ enabled: open });
  const { batchAddWorkspaceMembers, isBatchAddingWorkspaceMembers } = useWorkspaceMemberActions();
  const { locale } = useLocale();

  const joinedWorkspaceIds = useMemo(
    () => new Set(member?.joined_workspaces?.map(workspace => workspace.workspace_id) ?? []),
    [member?.joined_workspaces]
  );

  const assignableWorkspaces = useMemo(
    () => workspaces.filter(workspace => !joinedWorkspaceIds.has(workspace.id)),
    [joinedWorkspaceIds, workspaces]
  );

  const workspaceAdminRole = useMemo(() => roles.find(isAssignableWorkspaceAdminRole), [roles]);

  const selectableRoles = useMemo(
    () => roles.filter(isSelectableWorkspacePermissionTemplate),
    [roles]
  );

  const defaultRoleId = useMemo(
    () =>
      selectableRoles.find(role => role.system_key === 'default_basic')?.id ||
      selectableRoles.find(role => role.name.toLowerCase() === 'member')?.id ||
      selectableRoles[0]?.id ||
      '',
    [selectableRoles]
  );

  const getRoleDisplayName = useCallback(
    (role: (typeof selectableRoles)[number]) =>
      role.name_i18n ? pickLocale(role.name_i18n, locale, role.name) : role.name,
    [locale]
  );

  const effectiveTotal = Math.max(
    total - joinedWorkspaceIds.size,
    assignableWorkspaces.length + (hasMore ? 1 : 0)
  );
  const totalPages = Math.max(1, Math.ceil(effectiveTotal / pageSize));
  const isBusy = isBatchAddingWorkspaceMembers;
  const shouldShowLoading = isLoadingWorkspaces || isPlaceholderData;
  const selectedWorkspace = assignableWorkspaces.find(workspace => workspace.id === selectedWorkspaceId);

  useEffect(() => {
    setCurrentPage(1);
  }, [debouncedSearchKeyword]);

  useEffect(() => {
    if (!open) return;
    setSearchKeyword('');
    setSelectedWorkspaceId('');
    setShouldSetAdmin(false);
    setCurrentPage(1);
  }, [open, member?.account_id]);

  useEffect(() => {
    if (!open || shouldSetAdmin || selectedRoleId || !defaultRoleId) return;
    setSelectedRoleId(defaultRoleId);
  }, [defaultRoleId, open, selectedRoleId, shouldSetAdmin]);

  useEffect(() => {
    if (!open || shouldSetAdmin || !selectedRoleId) return;
    if (selectableRoles.length === 0) {
      setSelectedRoleId('');
      return;
    }
    if (!selectableRoles.some(role => role.id === selectedRoleId)) {
      setSelectedRoleId(defaultRoleId);
    }
  }, [defaultRoleId, open, selectableRoles, selectedRoleId, shouldSetAdmin]);

  useEffect(() => {
    if (!selectedWorkspaceId) return;
    if (!assignableWorkspaces.some(workspace => workspace.id === selectedWorkspaceId)) {
      setSelectedWorkspaceId('');
    }
  }, [assignableWorkspaces, selectedWorkspaceId]);

  const handleClose = () => {
    if (isBusy) return;
    onOpenChange(false);
  };

  const handleSubmit = async () => {
    const roleId = shouldSetAdmin ? workspaceAdminRole?.id : selectedRoleId;
    if (!member || !selectedWorkspaceId || !roleId) return;

    const result = await batchAddWorkspaceMembers({
      workspaceId: selectedWorkspaceId,
      data: {
        account_ids: [member.account_id],
        role_id: roleId,
      },
    });

    const added = result?.added_count ?? 0;
    const failed = result?.failed_count ?? 0;
    const skipped = result?.skipped_count ?? 0;
    if (failed > 0 || added === 0) {
      toast.error(t('resultWarning', { added, skipped, failed }));
      return;
    }

    toast.success(
      t('resultSuccess', {
        member: member.member_name || member.account_name,
        workspace: selectedWorkspace?.name || '',
      })
    );
    onAssigned?.();
    onOpenChange(false);
  };

  const renderWorkspace = (workspace: WorkspaceManagement) => {
    const selected = selectedWorkspaceId === workspace.id;
    return (
      <button
        key={workspace.id}
        type="button"
        onClick={() => setSelectedWorkspaceId(workspace.id)}
        className={cn(
          'flex w-full items-center justify-between gap-4 rounded-md border px-3 py-3 text-left transition-colors',
          selected
            ? 'border-primary bg-primary/5 text-primary'
            : 'border-border bg-background hover:border-primary/40 hover:bg-bg-canvas/60'
        )}
      >
        <div className="min-w-0">
          <span className="block truncate text-sm font-semibold">{workspace.name}</span>
          <div className="mt-1 flex items-center gap-3 text-xs text-muted-foreground">
            <span>{workspace.leader_name || t('noLeader')}</span>
            <span>
              {t('memberCount', {
                count: workspace.member_count ?? 0,
              })}
            </span>
          </div>
        </div>
        <div
          className={cn(
            'h-4 w-4 rounded-full border',
            selected ? 'border-primary bg-primary shadow-[inset_0_0_0_4px_var(--background)]' : ''
          )}
        />
      </button>
    );
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="flex max-h-[82vh] w-[720px] max-w-[calc(100vw-32px)] flex-col overflow-hidden">
        <DialogHeader>
          <DialogTitle>{t('title')}</DialogTitle>
          <DialogDescription>
            {member
              ? t('description', {
                  member: member.member_name || member.account_name,
                })
              : t('descriptionFallback')}
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="flex min-h-0 flex-1 flex-col gap-4">
          {member ? (
            <div className="rounded-md border border-border bg-bg-canvas/40 px-3 py-2">
              <div className="text-sm font-semibold text-text-primary">
                {member.member_name || member.account_name}
              </div>
              <div className="mt-0.5 text-xs text-muted-foreground">{member.account_email}</div>
            </div>
          ) : null}

          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_220px]">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={searchKeyword}
                onChange={event => setSearchKeyword(event.target.value)}
                placeholder={t('searchPlaceholder')}
                className="h-10 pl-9"
              />
            </div>
            {shouldSetAdmin ? (
              <div className="flex h-10 items-center justify-between gap-2 rounded-md border border-primary/20 bg-primary/5 px-3">
                <span className="truncate text-sm font-medium text-text-primary">
                  {t('assignAsAdmin')}
                </span>
                <Badge variant="secondary" className="shrink-0 rounded-md">
                  {workspaceAdminRole ? getRoleDisplayName(workspaceAdminRole) : '-'}
                </Badge>
              </div>
            ) : (
              <Select
                value={selectedRoleId}
                onValueChange={setSelectedRoleId}
                disabled={isLoadingRoles || isBusy}
              >
                <SelectTrigger className="h-10">
                  <SelectValue placeholder={t('rolePlaceholder')} />
                </SelectTrigger>
                <SelectContent>
                  {selectableRoles.map(role => (
                    <SelectItem key={role.id} value={role.id}>
                      {getRoleDisplayName(role)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          </div>

          <div className="rounded-md border border-border bg-bg-canvas/40 px-3 py-2">
            <div className="flex items-center justify-between gap-4">
              <div className="min-w-0">
                <div className="text-sm font-medium text-text-primary">
                  {t('adminSwitchLabel')}
                </div>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {workspaceAdminRole ? t('adminSwitchDescription') : t('adminRoleMissing')}
                </p>
              </div>
              <Switch
                checked={shouldSetAdmin}
                onCheckedChange={setShouldSetAdmin}
                disabled={isLoadingRoles || isBusy || !workspaceAdminRole}
              />
            </div>
          </div>

          <div className="min-h-[260px] flex-1 overflow-y-auto rounded-md border border-border/80 p-2">
            {shouldShowLoading ? (
              <div className="space-y-2">
                {Array.from({ length: 5 }).map((_, index) => (
                  <Skeleton key={index} className="h-[68px] rounded-md" />
                ))}
              </div>
            ) : assignableWorkspaces.length > 0 ? (
              <div className="space-y-2">{assignableWorkspaces.map(renderWorkspace)}</div>
            ) : (
              <div className="flex h-[240px] flex-col items-center justify-center text-center text-muted-foreground">
                <Users className="mb-3 h-10 w-10 opacity-30" />
                <p className="text-sm font-medium">{t('emptyTitle')}</p>
                <p className="mt-1 max-w-sm text-xs">{t('emptyDescription')}</p>
              </div>
            )}
          </div>

          <Pagination
            currentPage={currentPage}
            totalPages={totalPages}
            total={effectiveTotal}
            pageSize={pageSize}
            onPageChange={setCurrentPage}
            showJump={false}
            className="border-t pt-3"
          />
        </DialogBody>

        <DialogFooter>
          <Button variant="outline" onClick={handleClose} disabled={isBusy}>
            {t('cancel')}
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={
              !member ||
              !selectedWorkspaceId ||
              isBusy ||
              (shouldSetAdmin ? !workspaceAdminRole : !selectedRoleId)
            }
          >
            {isBusy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
            {t('confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
