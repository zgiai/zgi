'use client';

import { useT, type WorkspaceKey } from '@/i18n';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Badge } from '@/components/ui/badge';
import { TableCell } from '@/components/ui/table';
import { AlertCircle, Users, ShieldCheck, Loader2 } from 'lucide-react';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useAuthStore } from '@/store/auth-store';
import { useCurrentWorkspace } from '@/store/workspace-store';
import { useWorkspaceMemberActions } from '@/hooks/workspace/use-workspace-member-actions';
import { useEffect, useState } from 'react';
import { useOrganizationRoles } from '@/hooks/organization/use-organization-roles';
import { Button } from '@/components/ui/button';
import { Plus, Trash2 } from 'lucide-react';
import { AddWorkspaceMemberModal } from '@/components/member/add-workspace-member-modal';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import {
  useWorkspaceMembers,
  getWorkspaceMembersQueryKey,
} from '@/hooks/workspace/use-workspace-members';
import { useQueryClient } from '@tanstack/react-query';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Pagination } from '@/components/ui/pagination';
import { Skeleton } from '@/components/ui/skeleton';
import { StickyDataTable } from '@/components/common/sticky-data-table';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { WorkspaceMemberPermissionsDialog } from '@/components/member/workspace-member-permissions-dialog';
import type { WorkspaceMemberAccount } from '@/services/types/workspace';
import { useLocale } from '@/hooks/use-locale';
import { pickLocale } from '@/utils/tool-helpers';
import type { Role } from '@/services/types/organization';
import { PermissionDeniedState } from '@/components/common/permission-gate-state';

export default function WorkspaceMembersPage() {
  const t = useT();
  const [currentPage, setCurrentPage] = useState(1);
  const pageSize = 20;
  const {
    isWorkspaceManager,
    isLoading: isLoadingPermissions,
  } = useAccountPermissions();
  const canManageWorkspaceMembers = isWorkspaceManager();
  const { members, total, isLoading, error } = useWorkspaceMembers(undefined, undefined, {
    page: currentPage,
    limit: pageSize,
    enabled: canManageWorkspaceMembers,
  });
  const currentUser = useAuthStore.use.user();
  const currentWorkspace = useCurrentWorkspace();
  const {
    updateWorkspaceMemberRole: updateRole,
    updateWorkspaceMemberPermissions,
    removeWorkspaceMember: removeMember,
    isRemovingMember,
    isUpdatingPermissions,
    batchAddWorkspaceMembers,
    isBatchAddingWorkspaceMembers: isAddingMembers,
  } = useWorkspaceMemberActions();
  const { roles } = useOrganizationRoles({ enabled: canManageWorkspaceMembers });
  const { locale } = useLocale();

  // State for tracking which member's role is currently being updated (optimistic/UI state)
  const [updatingMemberId, setUpdatingMemberId] = useState<string | null>(null);
  const [addMemberDialogOpen, setAddMemberDialogOpen] = useState(false);
  const [permissionsDialogOpen, setPermissionsDialogOpen] = useState(false);
  const [memberToEditPermissions, setMemberToEditPermissions] =
    useState<WorkspaceMemberAccount | null>(null);

  // Get organization
  const { currentOrganization } = useOrganizations();

  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const canManageMembers = canManageWorkspaceMembers;
  const canManagePermissions = canManageWorkspaceMembers;
  const showActionsColumn = canManageMembers || canManagePermissions;
  const queryClient = useQueryClient();
  const isFixedGovernanceRole = (role?: string) => role === 'owner' || role === 'admin';
  const getRoleDisplayName = (role: Role) =>
    role.name_i18n ? pickLocale(role.name_i18n, locale, role.name) : role.name;

  useEffect(() => {
    if (currentPage > totalPages) {
      setCurrentPage(totalPages);
    }
  }, [currentPage, totalPages]);

  // Get status badge variant
  const getStatusVariant = (status: string) => {
    switch (status) {
      case 'active':
        return 'success';
      case 'pending':
        return 'secondary';
      case 'banned':
        return 'destructive';
      default:
        return 'outline';
    }
  };

  // Get role display text
  const getRoleText = (role?: string) => {
    const fallback = locale.toLowerCase().startsWith('zh') ? '成员' : 'Member';
    if (!role) return t('workspace.members.roles.normal') || fallback;
    return t(`workspace.members.roles.${role}` as WorkspaceKey) || fallback;
  };

  const getMemberPermissionDisplayName = (member: WorkspaceMemberAccount) => {
    if (isFixedGovernanceRole(member.role)) {
      return getRoleText(member.role) || member.role_name;
    }

    if (member.permission_source === 'direct') {
      return t('dashboard.organization.workspaceManagement.detail.memberPermissions.source.direct');
    }

    const templateId = member.permission_template_role_id || member.role_id;
    const matchedTemplate = roles.find(role => role.id === templateId);
    if (matchedTemplate) {
      return getRoleDisplayName(matchedTemplate);
    }

    if (member.permission_source === 'legacy_role') {
      return (
        member.role_name ||
        t('dashboard.organization.workspaceManagement.detail.memberPermissions.source.legacy')
      );
    }

    return member.role_name || getRoleText(member.role);
  };

  // Get status display text
  const getStatusText = (status: string) => {
    const statusKey = status as 'active' | 'pending' | 'banned' | 'uninitialized' | 'closed';
    return t(`workspace.members.statuses.${statusKey}` as WorkspaceKey);
  };

  // Get initials for avatar fallback
  const getInitials = (name: string) => {
    return name
      .split(' ')
      .map(n => n[0])
      .join('')
      .toUpperCase()
      .slice(0, 2);
  };

  const handleSaveMemberPermissions = async (memberId: string, permissions: string[]) => {
    if (!currentWorkspace?.id) return;
    await updateWorkspaceMemberPermissions({
      workspaceId: currentWorkspace.id,
      memberId,
      permissions,
    });
    await queryClient.invalidateQueries({
      queryKey: getWorkspaceMembersQueryKey(currentOrganization?.id ?? null, currentWorkspace.id),
    });
    setPermissionsDialogOpen(false);
    setMemberToEditPermissions(null);
  };

  const handleApplyMemberTemplate = async (memberId: string, roleId: string) => {
    if (!currentWorkspace?.id || !roleId) return;
    setUpdatingMemberId(memberId);
    try {
      await updateRole({
        workspaceId: currentWorkspace.id,
        memberId,
        role_id: roleId,
      });
      await queryClient.invalidateQueries({
        queryKey: getWorkspaceMembersQueryKey(currentOrganization?.id ?? null, currentWorkspace.id),
      });
      const appliedRole = roles.find(role => role.id === roleId);
      setMemberToEditPermissions(prev =>
        prev && prev.id === memberId
          ? {
              ...prev,
              role_id: roleId,
              role_name: appliedRole ? getRoleDisplayName(appliedRole) : prev.role_name,
              permissions: appliedRole?.permissions ?? prev.permissions,
              permission_source: 'role_template',
              permission_template_role_id: roleId,
            }
          : prev
      );
    } finally {
      setUpdatingMemberId(null);
    }
  };

  if (isLoadingPermissions) {
    return (
      <div className="flex h-full items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!canManageWorkspaceMembers) {
    return <PermissionDeniedState />;
  }

  return (
    <div className="mx-auto flex h-full max-w-7xl flex-col px-6 py-6">
      {/* Header */}
      <div className="mb-5 flex items-start justify-between gap-4">
        <div>
          <h2 className="text-2xl font-semibold tracking-tight text-foreground">
            {t('workspace.members.title')}
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">{t('workspace.members.description')}</p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {canManageMembers && (
            <Button size="sm" className="h-9 gap-2" onClick={() => setAddMemberDialogOpen(true)}>
              <Plus className="h-4 w-4" />
              {t('workspace.members.addMember')}
            </Button>
          )}
        </div>
      </div>

      {/* Error state */}
      {error && !isLoading && (
        <div className="flex flex-1 items-center justify-center rounded-lg border border-destructive/20 bg-destructive/5 py-12 text-destructive">
          <AlertCircle className="h-8 w-8" />
          <span className="ml-2">{t('workspace.members.error')}</span>
        </div>
      )}

      {/* Members table */}
      {!error && (
        <StickyDataTable
          className="min-h-[420px] flex-1 rounded-lg border border-border/80 bg-background shadow-sm"
          tableClassName="min-w-[760px] text-sm"
          headerClassName="[&_th]:h-10 [&_th]:text-xs [&_th]:normal-case [&_th]:tracking-normal"
          columns={[
            { key: 'name', header: t('workspace.members.name'), className: 'w-[220px] pl-4' },
            { key: 'email', header: t('workspace.members.email') },
            {
              key: 'department',
              header: t('workspace.members.department'),
              className: 'w-[100px]',
            },
            { key: 'role', header: t('workspace.members.role'), className: 'w-[150px]' },
            { key: 'status', header: t('workspace.members.status'), className: 'w-[100px]' },
            ...(showActionsColumn
              ? [
                  {
                    key: 'actions',
                    header: t('workspace.members.actions.header'),
                    className: 'w-[112px] pr-4',
                  },
                ]
              : []),
          ]}
          data={members}
          getRowKey={member => member.id}
          isLoading={isLoading}
          loadingRows={pageSize}
          renderSkeletonRow={index => (
            <tr key={`workspace-member-skeleton-${index}`} className="border-b border-border/10">
              <td colSpan={showActionsColumn ? 6 : 5} className="px-4 py-4">
                <Skeleton className="h-10 w-full rounded-xl opacity-60" />
              </td>
            </tr>
          )}
          emptyState={
            <div className="flex flex-col items-center justify-center flex-1 py-12 text-muted-foreground">
              <div className="mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-muted">
                <Users className="h-6 w-6" />
              </div>
              <span className="text-sm">{t('workspace.members.noMembers')}</span>
            </div>
          }
          pagination={
            <Pagination
              currentPage={currentPage}
              totalPages={totalPages}
              total={total}
              pageSize={pageSize}
              onPageChange={setCurrentPage}
              className="px-4 py-3 border-t border-border/40"
            />
          }
          renderRow={member => (
            <>
              <TableCell className="py-3.5 pl-4">
                <div className="flex items-center gap-3">
                  <Avatar className="h-8 w-8">
                    <AvatarImage
                      src={member.avatar_url || member.avatar || undefined}
                      alt={member.name}
                    />
                    <AvatarFallback className="text-xs">{getInitials(member.name)}</AvatarFallback>
                  </Avatar>
                  <span className="font-medium text-foreground">{member.name}</span>
                </div>
              </TableCell>
              <TableCell className="py-3.5 text-muted-foreground">{member.email}</TableCell>
              <TableCell className="py-3.5 text-muted-foreground">
                {member.department_name || '-'}
              </TableCell>
              <TableCell className="py-3.5">
                <Badge variant="outline" className="max-w-[140px] truncate">
                  {getMemberPermissionDisplayName(member)}
                </Badge>
              </TableCell>
              <TableCell className="py-3.5">
                <Badge variant={getStatusVariant(member.status)}>
                  {getStatusText(member.status)}
                </Badge>
              </TableCell>
              {showActionsColumn && (
                <TableCell className="py-3.5">
                  <div className="flex items-center justify-end gap-1">
                    {canManagePermissions &&
                    member.id !== currentUser?.id &&
                    !isFixedGovernanceRole(member.role) ? (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="ghost"
                            isIcon
                            aria-label={t(
                              'dashboard.organization.workspaceManagement.detail.memberPermissions.edit'
                            )}
                            title={t(
                              'dashboard.organization.workspaceManagement.detail.memberPermissions.edit'
                            )}
                            className="h-8 w-8 text-muted-foreground hover:text-primary"
                            onClick={() => {
                              setMemberToEditPermissions(member);
                              setPermissionsDialogOpen(true);
                            }}
                          >
                            <ShieldCheck className="h-4 w-4" />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent className="text-xs">
                          {t(
                            'dashboard.organization.workspaceManagement.detail.memberPermissions.edit'
                          )}
                        </TooltipContent>
                      </Tooltip>
                    ) : null}
                    {canManageMembers &&
                    member.id !== currentUser?.id &&
                    member.role !== 'owner' ? (
                      <ConfirmDialog
                        variant="danger"
                        title={t('workspace.members.removeMember.title')}
                        description={t('workspace.members.removeMember.description', {
                          name: member.name,
                        })}
                        confirmText={t('workspace.members.removeMember.confirm')}
                        cancelText={t('workspace.members.removeMember.cancel')}
                        loading={isRemovingMember && updatingMemberId === member.id}
                        onConfirm={() => {
                          if (!currentWorkspace?.id) return;
                          setUpdatingMemberId(member.id);
                          removeMember(
                            {
                              workspaceId: currentWorkspace.id,
                              memberId: member.id,
                            },
                            {
                              onSettled: () => setUpdatingMemberId(null),
                            }
                          );
                        }}
                        trigger={
                          <Button
                            variant="ghost"
                            isIcon
                            className="h-8 w-8 text-muted-foreground hover:text-destructive"
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        }
                      />
                    ) : null}
                  </div>
                </TableCell>
              )}
            </>
          )}
        />
      )}

      {/* Add Workspace Member Modal */}
      <AddWorkspaceMemberModal
        open={addMemberDialogOpen}
        onOpenChange={setAddMemberDialogOpen}
        workspaceId={currentWorkspace?.id || ''}
        workspaceName={currentWorkspace?.name}
        onAdd={async (memberIds: string[], roleId?: string) => {
          if (!currentWorkspace?.id) return;
          try {
            const result = await batchAddWorkspaceMembers({
              workspaceId: currentWorkspace.id,
              data: {
                account_ids: memberIds,
                role_id: roleId || '',
              },
            });
            // Invalidate members list cache to refresh data correctly
            queryClient.invalidateQueries({
              queryKey: getWorkspaceMembersQueryKey(
                currentOrganization?.id ?? null,
                currentWorkspace.id
              ),
            });
            return result;
          } catch (error) {
            console.error('Failed to add members:', error);
            throw error;
          }
        }}
        isLoading={isAddingMembers}
      />

      <WorkspaceMemberPermissionsDialog
        open={permissionsDialogOpen}
        onOpenChange={open => {
          setPermissionsDialogOpen(open);
          if (!open) setMemberToEditPermissions(null);
        }}
        member={memberToEditPermissions}
        onSave={handleSaveMemberPermissions}
        roleTemplates={roles}
        onApplyTemplate={handleApplyMemberTemplate}
        isSaving={isUpdatingPermissions}
        isApplyingTemplate={
          !!memberToEditPermissions && updatingMemberId === memberToEditPermissions.id
        }
      />
    </div>
  );
}
