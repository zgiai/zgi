'use client';

import { useState, useEffect } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { useT } from '@/i18n';
import {
  ChevronLeft,
  Pencil,
  Plus,
  User as UserIcon,
  Search,
  Trash2,
  Coins,
  Loader2,
  Check,
  X,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { TableCell, TableRow } from '@/components/ui/table';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
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
import { toast } from 'sonner';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useWorkspaceMembers } from '@/hooks/workspace/use-workspace-members';
import { useWorkspaceActions } from '@/hooks/workspace/use-workspace-actions';
import { useWorkspaceMemberActions } from '@/hooks/workspace/use-workspace-member-actions';
import type { WorkspaceMemberAccount } from '@/services/types/workspace';
import { AddWorkspaceMemberModal } from '@/components/member/add-workspace-member-modal';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useOrganizationRoles } from '@/hooks/organization/use-organization-roles';
import { useWorkspaceDetail } from '@/hooks/workspace/use-workspace-detail';
import { useWorkspaceStore } from '@/store/workspace-store';
import {
  useWorkspaceQuota,
  useUpdateWorkspaceQuota,
} from '@/hooks/workspace-quota/use-workspace-quota';
import { WorkspaceQuotaType } from '@/services/types/workspace-quota';
import { DEFAULT_AI_CREDIT_EDIT_MAX, sanitizeAiCreditIntegerInput } from '@/utils/ai-credits';
import { Pagination } from '@/components/ui/pagination';
import { StickyDataTable } from '@/components/common/sticky-data-table';

export default function WorkspaceDetailPage() {
  const params = useParams();
  const router = useRouter();
  const workspaceId = typeof params?.workspaceId === 'string' ? params.workspaceId : '';
  const t = useT('dashboard.organization.workspaceManagement');

  // Get organization
  const { currentOrganization } = useOrganizations();

  // Get roles for organization
  const { roles, isLoading: isLoadingRoles } = useOrganizationRoles();

  // Get workspace detail
  const {
    workspaceDetail: workspaceInfo,
    isLoading: isLoadingWorkspaces,
    refetch: refetchWorkspaceDetail,
  } = useWorkspaceDetail(workspaceId);

  // State
  const [memberSearchKeyword, setMemberSearchKeyword] = useState('');
  const [memberToRemove, setMemberToRemove] = useState<string | null>(null);
  const [updatingMemberId, setUpdatingMemberId] = useState<string | null>(null);
  const [memberPage, setMemberPage] = useState(1);
  const [isMemberPageChanging, setIsMemberPageChanging] = useState(false);
  const memberPageSize = 10;
  const debouncedMemberSearchKeyword = useDebouncedValue(memberSearchKeyword, 500);

  // Get workspace detail
  const {
    members: workspaceMembers,
    total: workspaceMembersTotal,
    page: workspaceMembersPage,
    isLoading: isLoadingMembers,
    isPlaceholderData: isPlaceholderMembers,
    refetch: refetchMembers,
  } = useWorkspaceMembers(currentOrganization?.id || null, workspaceId, {
    keyword: debouncedMemberSearchKeyword,
    page: memberPage,
    limit: memberPageSize,
    keepPreviousData: true,
  });

  // Workspace quota
  const {
    quota,
    isLoading: isLoadingQuota,
    refetch: refetchQuota,
  } = useWorkspaceQuota(workspaceId);
  const { updateQuota, isUpdating: isUpdatingQuota } = useUpdateWorkspaceQuota();

  // Mutations
  const { updateWorkspace } = useWorkspaceActions();
  const {
    removeWorkspaceMember,
    isRemovingMember,
    updateWorkspaceMemberRole: updateMemberRole,
    batchAddWorkspaceMembers,
    isBatchAddingWorkspaceMembers: isAddingMembers,
  } = useWorkspaceMemberActions();
  const currentWorkspace = useWorkspaceStore.use.currentWorkspace();
  const setCurrentWorkspace = useWorkspaceStore.use.setCurrentWorkspace();
  const workspacesFromStore = useWorkspaceStore.use.workspaces();
  const setWorkspaces = useWorkspaceStore.use.setWorkspaces();

  // State
  const [workspaceName, setWorkspaceName] = useState(workspaceInfo?.name || '');
  const [isEditingName, setIsEditingName] = useState(false);
  const [addMemberDialogOpen, setAddMemberDialogOpen] = useState(false);

  // Quota management state
  const [isEditingQuota, setIsEditingQuota] = useState(false);
  const [quotaType, setQuotaType] = useState<WorkspaceQuotaType>(WorkspaceQuotaType.Unlimited);
  const [remainQuota, setRemainQuota] = useState('');
  const tWorkspace = useT('workspace');
  const remainQuotaMaxLabel = DEFAULT_AI_CREDIT_EDIT_MAX.toLocaleString();

  const [removeMemberDialogOpen, setRemoveMemberDialogOpen] = useState(false);
  const workspaceMembersTotalPages = Math.max(1, Math.ceil(workspaceMembersTotal / memberPageSize));
  const shouldShowMemberSkeleton = isLoadingMembers || isMemberPageChanging;

  // Update workspace name when workspaceInfo changes
  useEffect(() => {
    if (workspaceInfo?.name) {
      setWorkspaceName(workspaceInfo.name);
    }
  }, [workspaceInfo?.name]);

  useEffect(() => {
    setIsMemberPageChanging(true);
    setMemberPage(1);
  }, [debouncedMemberSearchKeyword, workspaceId]);

  useEffect(() => {
    if (memberPage > workspaceMembersTotalPages) {
      setMemberPage(workspaceMembersTotalPages);
    }
  }, [memberPage, workspaceMembersTotalPages]);

  useEffect(() => {
    if (!isPlaceholderMembers && workspaceMembersPage === memberPage) {
      setIsMemberPageChanging(false);
    }
  }, [isPlaceholderMembers, memberPage, workspaceMembersPage]);

  const handleMemberPageChange = (page: number) => {
    if (page === memberPage) return;
    setIsMemberPageChanging(true);
    setMemberPage(page);
  };

  // Handle save workspace name
  const handleSaveWorkspaceName = async () => {
    const trimmedName = workspaceName.trim();
    if (!workspaceId || !trimmedName || !currentOrganization?.id) return;

    // Skip if name hasn't changed
    if (trimmedName === workspaceInfo?.name) {
      setIsEditingName(false);
      return;
    }

    try {
      await updateWorkspace({
        workspaceId: workspaceId,
        data: { name: trimmedName },
      });

      // Update local store if current workspace matched
      const newName = workspaceName.trim();
      if (currentWorkspace?.id === workspaceId) {
        setCurrentWorkspace({ ...currentWorkspace, name: newName });
      }
      setWorkspaces(
        workspacesFromStore.map(w => (w.id === workspaceId ? { ...w, name: newName } : w))
      );

      setIsEditingName(false);
      refetchWorkspaceDetail();
    } catch (error) {
      console.error('Failed to update workspace name:', error);
    }
  };

  // Handle remove member
  const handleRemoveMember = async () => {
    if (!workspaceId || !memberToRemove || !currentOrganization?.id) return;
    try {
      await removeWorkspaceMember({
        workspaceId: workspaceId,
        memberId: memberToRemove,
      });
      setRemoveMemberDialogOpen(false);
      setMemberToRemove(null);
      await refetchMembers();
    } catch (error) {
      console.error('Failed to remove member:', error);
    }
  };

  // Show skeleton until workspace detail and members list are both loaded (first time only)
  const isLoading = isLoadingWorkspaces || (isLoadingMembers && !workspaceMembers);

  if (isLoading) {
    return (
      <div className="flex flex-col h-full p-6 space-y-6 overflow-auto">
        {/* Header Skeleton */}
        <div className="flex items-center gap-4 justify-between">
          <div className="flex items-center gap-2">
            <Skeleton className="h-9 w-24" />
            <Skeleton className="h-8 w-48" />
            <Skeleton className="h-8 w-8 rounded" />
          </div>
          <div className="flex items-center gap-2">
            <Skeleton className="h-4 w-16" />
            <Skeleton className="h-9 w-32" />
          </div>
        </div>

        {/* Card Skeleton */}
        <div className="flex-1">
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <Skeleton className="h-6 w-24" />
                <Skeleton className="h-9 w-32" />
              </div>
            </CardHeader>
            <CardContent>
              <div className="space-y-3">
                {[...Array(5)].map((_, i) => (
                  <Skeleton key={i} className="h-16 w-full" />
                ))}
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    );
  }

  if (!workspaceInfo) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center">
          <p className="text-lg font-semibold">{t('detail.workspaceNotFound')}</p>
          <Button onClick={() => router.back()} className="mt-4">
            {t('detail.backToList')}
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col bg-bg-canvas/50 overflow-hidden">
      <header className="px-6 lg:px-8 py-5 flex items-center justify-between border-b border-border/40 bg-card/40 backdrop-blur-md sticky top-0 z-20">
        <div className="flex items-center gap-4">
          <Button
            variant="ghost"
            isIcon
            className="h-9 w-9 rounded-xl transition-all border shadow-sm"
            onClick={() => router.back()}
          >
            <ChevronLeft className="h-5 w-5" />
          </Button>
          <div className="flex-1">
            <div className="flex items-center gap-3">
              {isEditingName ? (
                <div className="flex items-center gap-3">
                  <Input
                    value={workspaceName}
                    onChange={e => setWorkspaceName(e.target.value)}
                    className="h-8 w-64 bg-muted/50 border focus:ring-1 focus:ring-brand-main/40 font-bold"
                    onBlur={handleSaveWorkspaceName}
                    onKeyDown={e => e.key === 'Enter' && handleSaveWorkspaceName()}
                    autoFocus
                  />
                </div>
              ) : (
                <div className="flex items-center gap-2 group">
                  <h1 className="text-xl font-bold text-text-primary tracking-tight">
                    {workspaceInfo?.name}
                  </h1>
                  <Button
                    variant="ghost"
                    isIcon
                    className="h-7 w-7 rounded-md opacity-0 group-hover:opacity-100 transition-opacity"
                    onClick={() => setIsEditingName(true)}
                  >
                    <Pencil className="h-3.5 w-3.5" />
                  </Button>
                </div>
              )}
            </div>
          </div>
        </div>
        <div className="flex items-center gap-3">
          <Button
            className="h-9 rounded-lg bg-primary hover:bg-primary-hover text-primary-foreground shadow-button-primary transition-all px-4 text-xs font-semibold"
            onClick={() => setAddMemberDialogOpen(true)}
          >
            <Plus className="h-3.5 w-3.5 mr-2" />
            {t('detail.addMember')}
          </Button>
        </div>
      </header>

      <main className="flex-1 overflow-y-auto p-4 lg:p-8 space-y-6 scrollbar-thin scrollbar-thumb-muted-foreground/10">
        <div className="space-y-6">
          {/* Left-Right Layout: Quota Card (left) + Members Table (right) */}
          <div className="flex gap-6">
            {/* Quota Management Card - Left */}
            <div>
              <Card className="glass-panel rounded-2xl border-none shadow-sm bg-card/40 w-[320px]">
                <CardHeader className="px-5 py-4 border-b border-border/40 bg-card/40 backdrop-blur-sm">
                  <div className="flex items-center justify-between">
                    <h3 className="text-sm font-bold text-text-primary uppercase tracking-wider flex items-center gap-2">
                      <Coins className="h-4 w-4" />
                      {tWorkspace('quota.title')}
                    </h3>
                    <Button
                      variant="ghost"
                      size="sm"
                      isIcon
                      onClick={() => {
                        setQuotaType(
                          quota?.quota_limit === null || quota?.quota_limit === undefined
                            ? WorkspaceQuotaType.Unlimited
                            : WorkspaceQuotaType.Custom
                        );
                        setRemainQuota(quota?.remain_quota?.toString() ?? '');
                        setIsEditingQuota(true);
                      }}
                    >
                      <Pencil className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                </CardHeader>
                <CardContent className="p-5">
                  {isLoadingQuota ? (
                    <div className="space-y-4">
                      <Skeleton className="h-12 w-full" />
                      <Skeleton className="h-12 w-full" />
                    </div>
                  ) : quota?.quota_limit === null || quota?.quota_limit === undefined ? (
                    // Unlimited type: show unlimited label
                    <div className="flex flex-col p-3 rounded-lg bg-muted/30">
                      <span className="text-xs text-muted-foreground mb-1">
                        {tWorkspace('quota.quotaLimit')}
                      </span>
                      <span className="text-xl font-bold">{tWorkspace('quota.unlimited')}</span>
                    </div>
                  ) : (
                    // Custom type: show used and remain quota only
                    <div className="space-y-4">
                      <div className="flex flex-col p-3 rounded-lg bg-orange-50 dark:bg-orange-950/20">
                        <span className="text-xs text-muted-foreground mb-1">
                          {tWorkspace('quota.usedQuota')}
                        </span>
                        <span className="text-xl font-bold text-orange-600">
                          {(quota?.used_quota ?? 0).toLocaleString()}
                        </span>
                      </div>
                      <div className="flex flex-col p-3 rounded-lg bg-green-50 dark:bg-green-950/20">
                        <span className="text-xs text-muted-foreground mb-1">
                          {tWorkspace('quota.remainQuota')}
                        </span>
                        <span className="text-xl font-bold text-green-600">
                          {(quota?.remain_quota ?? 0).toLocaleString()}
                        </span>
                      </div>
                    </div>
                  )}
                </CardContent>
              </Card>
            </div>

            {/* Members Table - Right */}
            <div className="glass-panel rounded-2xl overflow-hidden border-none shadow-sm bg-card/40 grow">
              <div className="px-6 py-5 border-b border-border/40 bg-card/40 backdrop-blur-sm flex items-center justify-between">
                <h3 className="text-sm font-bold text-text-primary uppercase tracking-wider flex items-center gap-2">
                  {t('detail.members')}
                  {workspaceMembersTotal > 0 && (
                    <span className="text-[11px] font-bold text-text-placeholder bg-muted/80 px-2 py-0.5 rounded-full uppercase tracking-wider">
                      {workspaceMembersTotal}
                    </span>
                  )}
                </h3>
                <div className="relative w-64">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
                  <Input
                    placeholder={t('detail.searchMembers')}
                    className="pl-9 h-9 bg-muted/50 border focus:ring-0 focus:border-brand-main/40 transition-all text-xs"
                    value={memberSearchKeyword}
                    onChange={e => setMemberSearchKeyword(e.target.value)}
                  />
                </div>
              </div>

              <StickyDataTable
                columns={[
                  { key: 'name', header: t('detail.nameAndEmail'), className: 'pl-8' },
                  { key: 'role', header: t('detail.workspaceRole') },
                  { key: 'department', header: t('detail.department') },
                  {
                    key: 'operations',
                    header: t('detail.operations'),
                    align: 'right',
                    className: 'pr-8',
                  },
                ]}
                data={workspaceMembers || []}
                getRowKey={member => member.id}
                isLoading={shouldShowMemberSkeleton}
                loadingRows={5}
                scrollClassName="scrollbar-thumb-muted-foreground/10 min-h-[400px]"
                renderSkeletonRow={index => (
                  <TableRow key={`member-skeleton-${index}`} className="border-b border-border/10">
                    <TableCell className="py-4 pl-8">
                      <div className="flex items-center gap-3">
                        <Skeleton className="h-9 w-9 rounded-xl" />
                        <div className="space-y-2">
                          <Skeleton className="h-4 w-28" />
                          <Skeleton className="h-3 w-40" />
                        </div>
                      </div>
                    </TableCell>
                    <TableCell className="py-4">
                      <Skeleton className="h-8 w-32 rounded-lg" />
                    </TableCell>
                    <TableCell className="py-4">
                      <Skeleton className="h-4 w-24" />
                    </TableCell>
                    <TableCell className="py-4 pr-8">
                      <div className="flex justify-end">
                        <Skeleton className="h-8 w-8 rounded-lg" />
                      </div>
                    </TableCell>
                  </TableRow>
                )}
                emptyState={
                  <div className="flex flex-col items-center justify-center flex-1 py-24 text-muted-foreground">
                    <UserIcon className="h-12 w-12 mb-4" />
                    <p className="text-sm font-medium">{t('detail.noMembers')}</p>
                  </div>
                }
                pagination={
                  <Pagination
                    currentPage={memberPage}
                    totalPages={workspaceMembersTotalPages}
                    total={workspaceMembersTotal}
                    pageSize={memberPageSize}
                    onPageChange={handleMemberPageChange}
                    className="px-6 py-4 border-t border-border/40"
                  />
                }
              >
                {!shouldShowMemberSkeleton &&
                  workspaceMembers?.map((member: WorkspaceMemberAccount) => (
                    <TableRow
                      key={member.id}
                      className="group border-b border-border/10 hover:bg-bg-canvas/40 transition-all duration-200 interactive-subtle"
                    >
                      <TableCell className="py-4 pl-8">
                        <div className="flex items-center gap-3">
                          <div className="w-9 h-9 rounded-xl bg-primary text-primary-foreground flex items-center justify-center font-bold text-xs shadow-sm">
                            {member.name.charAt(0).toUpperCase()}
                          </div>
                          <div className="flex flex-col gap-0.5">
                            <span className="font-semibold text-text-primary text-[13px] group-hover:text-primary transition-colors">
                              {member.member_name || member.name}
                            </span>
                            <span className="text-[11px] text-text-placeholder font-medium">
                              {member.email}
                            </span>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell className="py-4">
                        {member.role === 'owner' ? (
                          <Badge
                            variant="secondary"
                            className="bg-primary/10 text-primary border-none text-[10px] font-bold px-2 py-0.5 rounded-md uppercase tracking-wider shadow-none"
                          >
                            {member.role_name}
                          </Badge>
                        ) : (
                          <Select
                            value={member.role_id || ''}
                            onValueChange={async value => {
                              try {
                                setUpdatingMemberId(member.id);
                                await updateMemberRole({
                                  workspaceId: workspaceId,
                                  memberId: member.id,
                                  role_id: value,
                                });
                              } catch (error) {
                                console.error('Failed to update member role:', error);
                              } finally {
                                setUpdatingMemberId(null);
                              }
                            }}
                            disabled={isLoadingRoles || updatingMemberId === member.id}
                          >
                            <SelectTrigger className="h-8 w-32 bg-muted/60 border text-xs font-semibold rounded-lg shadow-none focus:ring-1 focus:ring-brand-main/20">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent className="glass-panel border-none">
                              {isLoadingRoles ? (
                                <SelectItem value="" disabled className="text-xs">
                                  {t('detail.loading')}
                                </SelectItem>
                              ) : (
                                roles
                                  .filter(
                                    role =>
                                      role.status === 'active' &&
                                      role.id.toLowerCase() !== 'owner' &&
                                      role.name.toLowerCase() !== 'owner'
                                  )
                                  .map(role => (
                                    <SelectItem key={role.id} value={role.id} className="text-xs">
                                      {role.name}
                                    </SelectItem>
                                  ))
                              )}
                            </SelectContent>
                          </Select>
                        )}
                      </TableCell>
                      <TableCell className="py-4 text-[13px] text-text-secondary font-medium">
                        {member.department_name || '-'}
                      </TableCell>
                      <TableCell className="py-4 pr-8 text-right">
                        {member.role !== 'owner' && (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Button
                                variant="ghost"
                                size="sm"
                                isIcon
                                className="h-8 w-8 rounded-lg text-text-placeholder hover:bg-destructive hover:text-destructive-foreground transition-all shadow-none opacity-0 group-hover:opacity-100"
                                onClick={() => {
                                  setMemberToRemove(member.id);
                                  setRemoveMemberDialogOpen(true);
                                }}
                              >
                                <Trash2 className="h-4 w-4" />
                              </Button>
                            </TooltipTrigger>
                            <TooltipContent className="glass-panel border-none text-xs">
                              {t('detail.remove')}
                            </TooltipContent>
                          </Tooltip>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
              </StickyDataTable>
            </div>
          </div>
        </div>
      </main>

      {/* Dialogs */}
      <ConfirmDialog
        open={removeMemberDialogOpen}
        onOpenChange={setRemoveMemberDialogOpen}
        title={t('detail.removeConfirm.title')}
        description={t('detail.removeConfirm.description')}
        confirmText={t('detail.removeConfirm.confirm')}
        cancelText={t('detail.removeConfirm.cancel')}
        loading={isRemovingMember}
        onConfirm={handleRemoveMember}
        variant="danger"
      />

      <AddWorkspaceMemberModal
        open={addMemberDialogOpen}
        onOpenChange={setAddMemberDialogOpen}
        workspaceId={workspaceId}
        workspaceName={workspaceInfo?.name || ''}
        onAdd={async (memberIds: string[], roleId?: string) => {
          if (!workspaceId) return;
          try {
            const result = await batchAddWorkspaceMembers({
              workspaceId,
              data: {
                account_ids: memberIds,
                role_id: roleId || '',
              },
            });
            await refetchMembers();
            return result;
          } catch (error) {
            console.error('Failed to add members:', error);
            throw error;
          }
        }}
        isLoading={isAddingMembers}
      />

      {/* Quota Edit Dialog */}
      <Dialog open={isEditingQuota} onOpenChange={setIsEditingQuota}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Coins className="h-5 w-5" />
              {tWorkspace('quota.title')}
            </DialogTitle>
            <DialogDescription>{tWorkspace('quota.editDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-3">
            <div className="flex flex-col space-y-2">
              <label className="text-sm font-medium">{tWorkspace('quota.quotaLimit')}</label>
              <Select value={quotaType} onValueChange={v => setQuotaType(v as WorkspaceQuotaType)}>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={WorkspaceQuotaType.Unlimited}>
                    {tWorkspace('quota.unlimited')}
                  </SelectItem>
                  <SelectItem value={WorkspaceQuotaType.Custom}>
                    {tWorkspace('quota.custom')}
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>
            {quotaType === WorkspaceQuotaType.Custom && (
              <div className="flex flex-col space-y-2">
                <label className="text-sm font-medium">{tWorkspace('quota.remainQuota')}</label>
                <Input
                  type="number"
                  value={remainQuota}
                  onChange={e =>
                    setRemainQuota(
                      sanitizeAiCreditIntegerInput(e.target.value, DEFAULT_AI_CREDIT_EDIT_MAX)
                    )
                  }
                  placeholder="1000"
                  min={0}
                  max={DEFAULT_AI_CREDIT_EDIT_MAX}
                />
                <p className="text-xs text-muted-foreground">
                  {tWorkspace('quota.hints.remainQuotaMax', { max: remainQuotaMaxLabel })}
                </p>
              </div>
            )}
          </DialogBody>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setIsEditingQuota(false)}
              disabled={isUpdatingQuota}
            >
              <X className="h-4 w-4 mr-1" />
              {t('detail.cancel')}
            </Button>
            <Button
              onClick={async () => {
                const remainValue =
                  remainQuota.trim() === '' ? 0 : Number.parseInt(remainQuota, 10);
                const usedValue = quota?.used_quota ?? 0;
                if (remainValue > DEFAULT_AI_CREDIT_EDIT_MAX) {
                  toast.error(
                    tWorkspace('quota.validation.remainQuotaMax', {
                      max: remainQuotaMaxLabel,
                    })
                  );
                  return;
                }
                await updateQuota(workspaceId, {
                  quota_type: quotaType,
                  // For custom type: quota_amount = remain_quota + used_quota
                  quota_amount:
                    quotaType === WorkspaceQuotaType.Custom ? remainValue + usedValue : undefined,
                  remain_quota: quotaType === WorkspaceQuotaType.Custom ? remainValue : undefined,
                });
                setIsEditingQuota(false);
                refetchQuota();
              }}
              disabled={isUpdatingQuota}
            >
              {isUpdatingQuota ? (
                <Loader2 className="h-4 w-4 mr-1 animate-spin" />
              ) : (
                <Check className="h-4 w-4 mr-1" />
              )}
              {t('detail.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
