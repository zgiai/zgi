'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { useT } from '@/i18n';
import {
  AlertTriangle,
  Check,
  Loader2,
  LogOut,
  Pencil,
  Search,
  ShieldCheck,
  X,
} from 'lucide-react';
import { useCurrentWorkspace, useWorkspaceStore } from '@/store/workspace-store';
import { useWorkspaceActions } from '@/hooks/workspace/use-workspace-actions';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useWorkspaceMembers } from '@/hooks/workspace/use-workspace-members';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useCurrentUser } from '@/store/auth-store';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Pagination } from '@/components/ui/pagination';
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';

export default function WorkspaceSettingsPage() {
  const t = useT('workspace');
  const tCommon = useT('common');
  const router = useRouter();

  const currentWorkspace = useCurrentWorkspace();
  const currentUser = useCurrentUser();
  const { currentOrganization } = useOrganizations();
  const { organizationRole, workspaceRole } = useAccountPermissions();
  const isOrganizationManager = organizationRole === 'owner' || organizationRole === 'admin';
  const canManage = isOrganizationManager || workspaceRole === 'owner' || workspaceRole === 'admin';
  const canTransferOwnership = isOrganizationManager || workspaceRole === 'owner';

  const {
    updateWorkspace,
    transferOwnership,
    leaveWorkspace,
    isUpdating,
    isTransferring,
    isLeaving,
  } = useWorkspaceActions();
  const workspaces = useWorkspaceStore.use.workspaces();
  const setWorkspaces = useWorkspaceStore.use.setWorkspaces();
  const setCurrentWorkspace = useWorkspaceStore.use.setCurrentWorkspace();

  // Workspace Name Editing State
  const [isEditingName, setIsEditingName] = useState(false);
  const [editedName, setEditedName] = useState('');

  // Transfer Ownership State
  const [selectedNewOwnerId, setSelectedNewOwnerId] = useState<string>('');
  const [selectedNewOwnerName, setSelectedNewOwnerName] = useState<string>('');
  const [ownerSearchKeyword, setOwnerSearchKeyword] = useState('');
  const [ownerPage, setOwnerPage] = useState(1);
  const ownerPageSize = 20;
  const debouncedOwnerSearchKeyword = useDebouncedValue(ownerSearchKeyword, 400);

  const {
    members,
    total: ownerCandidatesTotal,
    isLoading: isLoadingMembers,
  } = useWorkspaceMembers(currentOrganization?.id || null, currentWorkspace?.id || null, {
    keyword: debouncedOwnerSearchKeyword,
    page: ownerPage,
    limit: ownerPageSize,
    enabled: canTransferOwnership && !!currentWorkspace,
  });
  const { members: ownerSnapshotMembers } = useWorkspaceMembers(
    currentOrganization?.id || null,
    currentWorkspace?.id || null,
    {
      page: 1,
      limit: 100,
      enabled: canTransferOwnership && !!currentWorkspace,
    }
  );
  const ownerCandidatesTotalPages = Math.max(1, Math.ceil(ownerCandidatesTotal / ownerPageSize));

  useEffect(() => {
    if (currentWorkspace) {
      setEditedName(currentWorkspace.name);
    }
  }, [currentWorkspace]);

  useEffect(() => {
    setOwnerPage(1);
  }, [debouncedOwnerSearchKeyword, currentWorkspace?.id]);

  useEffect(() => {
    if (ownerPage > ownerCandidatesTotalPages) {
      setOwnerPage(ownerCandidatesTotalPages);
    }
  }, [ownerCandidatesTotalPages, ownerPage]);

  const handleUpdateName = async () => {
    if (
      !currentWorkspace ||
      !currentOrganization ||
      !editedName.trim() ||
      editedName === currentWorkspace.name
    ) {
      setIsEditingName(false);
      return;
    }

    try {
      await updateWorkspace({
        workspaceId: currentWorkspace.id,
        data: { name: editedName.trim() },
      });

      // Update local store
      const newName = editedName.trim();
      setCurrentWorkspace({ ...currentWorkspace, name: newName });
      setWorkspaces(
        workspaces.map(w => (w.id === currentWorkspace.id ? { ...w, name: newName } : w))
      );

      setIsEditingName(false);
    } catch (error) {
      console.error('Failed to update workspace name:', error);
    }
  };

  const handleTransferOwnership = async () => {
    if (!currentWorkspace || !currentOrganization || !selectedNewOwnerId) return;

    try {
      await transferOwnership({
        workspaceId: currentWorkspace.id,
        data: { new_owner_id: selectedNewOwnerId },
      });
      setSelectedNewOwnerId('');
      setSelectedNewOwnerName('');
    } catch (error) {
      console.error('Failed to transfer ownership:', error);
    }
  };

  const handleLeaveWorkspace = async () => {
    if (!currentWorkspace || !currentOrganization) return;

    try {
      await leaveWorkspace(currentWorkspace.id);
      // Redirect to console home after leaving
      router.push('/console');
    } catch (error) {
      console.error('Failed to leave workspace:', error);
    }
  };

  if (!currentWorkspace) {
    return (
      <div className="flex items-center justify-center py-20">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  const currentLeadId = currentWorkspace.leader_id || '';
  const currentLeadMember = ownerSnapshotMembers.find(
    m => (currentLeadId && m.id === currentLeadId) || m.role === 'owner'
  );
  const resolvedCurrentLeadId = currentLeadId || currentLeadMember?.id || '';
  const currentLeadName =
    currentLeadMember?.member_name ||
    currentLeadMember?.name ||
    currentWorkspace.leader_name ||
    currentLeadMember?.email ||
    resolvedCurrentLeadId;
  const currentLeadEmail = currentLeadMember?.email || '';
  const hasCurrentLead = !!currentLeadName;
  const otherMembers = members.filter(
    m => m.role !== 'owner' && (!resolvedCurrentLeadId || m.id !== resolvedCurrentLeadId)
  );
  const selectedNewOwner = otherMembers.find(m => m.id === selectedNewOwnerId);
  const isCurrentUserWorkspaceLead =
    !!currentUser?.id && !!resolvedCurrentLeadId && resolvedCurrentLeadId === currentUser.id;
  const isManagingByOrganization = isOrganizationManager && !isCurrentUserWorkspaceLead;

  return (
    <div className="mx-auto max-w-4xl px-4 py-5 @md/console:px-6 @md/console:py-6">
      {/* Header */}
      <div className="mb-5">
        <h2 className="text-2xl font-semibold tracking-tight text-foreground">
          {t('settings.title')}
        </h2>
        <p className="mt-1 text-sm text-muted-foreground">{t('settings.description')}</p>
      </div>

      <div className="grid gap-5">
        {/* Basic Information Card */}
        <Card className="border-border/80 shadow-sm">
          <CardHeader className="space-y-1.5">
            <CardTitle className="text-base">
              {t('settings.basicInfo.title') || 'Basic Information'}
            </CardTitle>
            <CardDescription>
              {t('settings.basicInfo.description') ||
                "View and manage your workspace's basic details."}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-5">
            <div className="flex flex-col space-y-1.5">
              <label className="text-sm font-medium leading-5 text-muted-foreground">
                {t('settings.basicInfo.workspaceName') || 'Workspace Name'}
              </label>
              <div className="flex min-h-9 items-center gap-2">
                {isEditingName ? (
                  <>
                    <Input
                      value={editedName}
                      onChange={e => setEditedName(e.target.value)}
                      className="h-9 max-w-md"
                      autoFocus
                    />
                    <Button
                      isIcon
                      variant="ghost"
                      onClick={handleUpdateName}
                      disabled={isUpdating || !editedName.trim()}
                    >
                      {isUpdating ? (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        <Check className="h-4 w-4 text-green-500" />
                      )}
                    </Button>
                    <Button
                      isIcon
                      variant="ghost"
                      onClick={() => {
                        setIsEditingName(false);
                        setEditedName(currentWorkspace.name);
                      }}
                      disabled={isUpdating}
                    >
                      <X className="h-4 w-4 text-destructive" />
                    </Button>
                  </>
                ) : (
                  <>
                    <span className="text-base font-medium leading-6 text-foreground">
                      {currentWorkspace.name}
                    </span>
                    {canManage && (
                      <Button
                        isIcon
                        variant="ghost"
                        onClick={() => setIsEditingName(true)}
                        className="h-8 w-8 text-muted-foreground hover:text-foreground"
                      >
                        <Pencil className="h-4 w-4" />
                      </Button>
                    )}
                  </>
                )}
              </div>
            </div>

            <div className="flex flex-col space-y-1.5">
              <label className="text-sm font-medium leading-5 text-muted-foreground">
                {t('settings.basicInfo.workspaceId') || 'Workspace ID'}
              </label>
              <span className="select-all font-mono text-sm leading-6 text-muted-foreground">
                {currentWorkspace.id}
              </span>
            </div>
          </CardContent>
        </Card>

        {/* Workspace Lead Card */}
        {canTransferOwnership && (
          <Card className="border-border/80 shadow-sm">
            <CardHeader className="space-y-1.5">
              <CardTitle className="flex items-center gap-2 text-base text-foreground">
                <ShieldCheck className="h-4 w-4 text-muted-foreground" />
                {t('settings.transfer.title') || 'Transfer Ownership'}
              </CardTitle>
              <CardDescription>
                {t('settings.transfer.description') ||
                  'Transfer this workspace to another member. This action cannot be undone.'}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-5">
              {isManagingByOrganization && (
                <div className="flex items-start gap-3 rounded-md border border-blue-200 bg-blue-50 px-3 py-3 text-sm text-blue-900">
                  <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0" />
                  <p className="leading-5">{t('settings.transfer.organizationManagerNotice')}</p>
                </div>
              )}

              <div className="rounded-md border bg-muted/20 px-4 py-3">
                <div className="mb-3 text-sm font-medium text-muted-foreground">
                  {t('settings.transfer.currentLead')}
                </div>
                {hasCurrentLead ? (
                  <div className="flex items-center gap-3">
                    <Avatar className="h-9 w-9">
                      <AvatarImage
                        src={
                          currentLeadMember?.avatar_url || currentLeadMember?.avatar || undefined
                        }
                      />
                      <AvatarFallback className="text-xs">
                        {(currentLeadName || '?').slice(0, 2).toUpperCase()}
                      </AvatarFallback>
                    </Avatar>
                    <div className="min-w-0">
                      <div className="truncate text-sm font-semibold text-foreground">
                        {currentLeadName}
                      </div>
                      {currentLeadEmail && (
                        <div className="truncate text-xs text-muted-foreground">
                          {currentLeadEmail}
                        </div>
                      )}
                    </div>
                  </div>
                ) : (
                  <div className="flex items-center gap-2 text-sm text-muted-foreground">
                    <AlertTriangle className="h-4 w-4" />
                    {t('settings.transfer.noCurrentLead')}
                  </div>
                )}
              </div>

              <div className="flex flex-col space-y-2">
                <label className="text-sm font-medium">
                  {t('settings.transfer.selectMember') || 'Select New Owner'}
                </label>
                <div className="space-y-3">
                  <div className="relative max-w-md">
                    <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                    <Input
                      value={ownerSearchKeyword}
                      onChange={event => setOwnerSearchKeyword(event.target.value)}
                      placeholder={t('settings.transfer.searchPlaceholder') || 'Search members'}
                      className="h-9 bg-background pl-9"
                    />
                  </div>
                  <div className="max-w-2xl space-y-3">
                    <div className="grid max-h-[260px] gap-2 overflow-y-auto rounded-md border bg-background p-2">
                      {isLoadingMembers ? (
                        <div className="flex items-center justify-center py-8">
                          <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
                        </div>
                      ) : otherMembers.length > 0 ? (
                        otherMembers.map(member => {
                          const displayName = member.member_name || member.name || member.email;
                          const isSelected = selectedNewOwnerId === member.id;
                          return (
                            <button
                              key={member.id}
                              type="button"
                              disabled={isTransferring}
                              onClick={() => {
                                setSelectedNewOwnerId(member.id);
                                setSelectedNewOwnerName(displayName);
                              }}
                              className={`flex w-full items-center gap-3 rounded-md border px-3 py-3 text-left transition-colors ${
                                isSelected
                                  ? 'border-primary bg-primary/5'
                                  : 'border-transparent hover:bg-muted/60'
                              }`}
                            >
                              <Avatar className="h-9 w-9">
                                <AvatarImage
                                  src={member.avatar_url || member.avatar || undefined}
                                />
                                <AvatarFallback className="text-xs">
                                  {(displayName || '?').slice(0, 2).toUpperCase()}
                                </AvatarFallback>
                              </Avatar>
                              <div className="min-w-0 flex-1">
                                <div className="truncate text-sm font-semibold text-foreground">
                                  {displayName}
                                </div>
                                <div className="truncate text-xs text-muted-foreground">
                                  {member.email}
                                </div>
                              </div>
                              {isSelected && <Check className="h-4 w-4 text-primary" />}
                            </button>
                          );
                        })
                      ) : (
                        <div className="px-3 py-8 text-center text-sm text-muted-foreground">
                          {t('settings.transfer.noMembers')}
                        </div>
                      )}
                    </div>
                    <Pagination
                      currentPage={ownerPage}
                      totalPages={ownerCandidatesTotalPages}
                      total={ownerCandidatesTotal}
                      pageSize={ownerPageSize}
                      onPageChange={setOwnerPage}
                      showJump={false}
                      className="max-w-md"
                    />
                    <ConfirmDialog
                      title={t('settings.transfer.confirm.title')}
                      description={t('settings.transfer.confirm.description', {
                        name:
                          selectedNewOwnerName ||
                          selectedNewOwner?.member_name ||
                          selectedNewOwner?.name ||
                          'the selected member',
                      })}
                      confirmText={t('settings.transfer.confirm.button') || 'Transfer'}
                      cancelText={tCommon('cancel')}
                      variant="warning"
                      loading={isTransferring}
                      onConfirm={handleTransferOwnership}
                      trigger={
                        <Button
                          variant="outline"
                          className="border-destructive/30 text-destructive hover:bg-destructive/5 hover:text-destructive"
                          disabled={!selectedNewOwnerId || isTransferring}
                        >
                          {isTransferring ? (
                            <Loader2 className="h-4 w-4 animate-spin mr-2" />
                          ) : null}
                          {t('settings.transfer.button') || 'Transfer'}
                        </Button>
                      }
                    />
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        )}

        {/* Leave Workspace Card */}
        {workspaceRole && !isCurrentUserWorkspaceLead && !isOrganizationManager && (
          <Card className="border-border/80 shadow-sm">
            <CardHeader className="space-y-1.5">
              <CardTitle className="flex items-center gap-2 text-base text-foreground">
                <LogOut className="h-4 w-4 text-muted-foreground" />
                {t('settings.leave.title') || 'Leave Workspace'}
              </CardTitle>
              <CardDescription>
                {t('settings.leave.description') ||
                  'Leave this workspace. You will lose access to its resources.'}
              </CardDescription>
            </CardHeader>
            <CardFooter>
              <ConfirmDialog
                title={t('settings.leave.confirm.title')}
                description={t('settings.leave.confirm.description')}
                confirmText={t('settings.leave.confirm.button') || 'Leave'}
                cancelText={tCommon('cancel')}
                variant="warning"
                loading={isLeaving}
                onConfirm={handleLeaveWorkspace}
                trigger={
                  <Button
                    variant="outline"
                    className="border-destructive/30 text-destructive hover:bg-destructive/5 hover:text-destructive"
                    disabled={isLeaving}
                  >
                    {isLeaving ? (
                      <Loader2 className="h-4 w-4 animate-spin mr-2" />
                    ) : (
                      <LogOut className="h-4 w-4 mr-2" />
                    )}
                    {t('settings.leave.button') || 'Leave Workspace'}
                  </Button>
                }
              />
            </CardFooter>
          </Card>
        )}
      </div>
    </div>
  );
}
